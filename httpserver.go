package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"runtime"
	"github.com/openatx/atx-agent/jsonrpc"
	"github.com/mholt/archiver"
	"github.com/gorilla/mux"
	"github.com/openatx/androidutils"
	"github.com/openatx/atx-agent/cmdctrl"
	"github.com/prometheus/procfs"
	"github.com/rs/cors"
)

type Server struct {
	// tunnel     *TunnelProxy
	httpServer *http.Server
}

func NewServer() *Server {
	server := &Server{}
	server.initHTTPServer()
	return server
}

func (server *Server) initHTTPServer() {
	m := mux.NewRouter()

	m.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "<html><head><title>BMW-Agent</title></head><body><h1>BMW-Agent is running</h1></body></html>")
	})

	m.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, version)
	})

	// jsonrpc client to call uiautomator
	rpcc := jsonrpc.NewClient("http://127.0.0.1:9008/jsonrpc/0")
	rpcc.ErrorCallback = func() error {
		service.Restart("uiautomator")
		return nil
	}
	rpcc.ErrorFixTimeout = 40 * time.Second
	rpcc.ServerOK = func() bool {
		return service.Running("uiautomator")
	}

	m.HandleFunc("/newCommandTimeout", func(w http.ResponseWriter, r *http.Request) {
		var timeout int
		err := json.NewDecoder(r.Body).Decode(&timeout) // TODO: auto get rotation
		if err != nil {
			http.Error(w, "Empty payload", 400) // bad request
			return
		}
		cmdTimeout := time.Duration(timeout) * time.Second
		uiautomatorTimer.Reset(cmdTimeout)
		renderJSON(w, map[string]interface{}{
			"success":     true,
			"description": fmt.Sprintf("newCommandTimeout updated to %v", cmdTimeout),
		})
	}).Methods("POST")

	// robust communicate with uiautomator
	// If the service is down, restart it and wait it recover
	m.HandleFunc("/dump/hierarchy", func(w http.ResponseWriter, r *http.Request) {
		if !service.Running("uiautomator") {
			xmlContent, err := dumpHierarchy()
			if err != nil {
				log.Println("Err:", err)
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			renderJSON(w, map[string]interface{}{
				"jsonrpc": "2.0",
				"id":      1,
				"result":  xmlContent,
			})
			return
		}
		resp, err := rpcc.RobustCall("dumpWindowHierarchy", false) // false: no compress
		if err != nil {
			log.Println("Err:", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		renderJSON(w, resp)
	})

	m.HandleFunc("/proc/list", func(w http.ResponseWriter, r *http.Request) {
		ps, err := listAllProcs()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		renderJSON(w, ps)
	})

	m.HandleFunc("/proc/{pkgname}/meminfo", func(w http.ResponseWriter, r *http.Request) {
		pkgname := mux.Vars(r)["pkgname"]
		info, err := parseMemoryInfo(pkgname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		renderJSON(w, info)
	})

	m.HandleFunc("/proc/{pkgname}/meminfo/all", func(w http.ResponseWriter, r *http.Request) {
		pkgname := mux.Vars(r)["pkgname"]
		ps, err := listAllProcs()
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}

		mems := make(map[string]map[string]int, 0)
		for _, p := range ps {
			if len(p.Cmdline) != 1 {
				continue
			}
			if p.Name == pkgname || strings.HasPrefix(p.Name, pkgname+":") {
				info, err := parseMemoryInfo(p.Name)
				if err != nil {
					continue
				}
				mems[p.Name] = info
			}
		}
		renderJSON(w, mems)
	})

	// make(map[int][]int)
	m.HandleFunc("/proc/{pkgname}/cpuinfo", func(w http.ResponseWriter, r *http.Request) {
		pkgname := mux.Vars(r)["pkgname"]
		pid, err := pidOf(pkgname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusGone)
			return
		}
		info, err := readCPUInfo(pid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		renderJSON(w, info)
	})

	m.HandleFunc("/webviews", func(w http.ResponseWriter, r *http.Request) {
		netUnix, err := procfs.NewNetUnix()
		if err != nil {
			return
		}

		unixPaths := make(map[string]bool, 0)
		for _, row := range netUnix.Rows {
			if !strings.HasPrefix(row.Path, "@") {
				continue
			}
			if !strings.Contains(row.Path, "devtools_remote") {
				continue
			}
			unixPaths[row.Path[1:]] = true
		}
		socketPaths := make([]string, 0, len(unixPaths))
		for key := range unixPaths {
			socketPaths = append(socketPaths, key)
		}
		renderJSON(w, socketPaths)
	})

	m.HandleFunc("/webviews/{pkgname}", func(w http.ResponseWriter, r *http.Request) {
		packageName := mux.Vars(r)["pkgname"]
		netUnix, err := procfs.NewNetUnix()
		if err != nil {
			return
		}

		unixPaths := make(map[string]bool, 0)
		for _, row := range netUnix.Rows {
			if !strings.HasPrefix(row.Path, "@") {
				continue
			}
			if !strings.Contains(row.Path, "devtools_remote") {
				continue
			}
			unixPaths[row.Path[1:]] = true
		}

		result := make([]interface{}, 0)
		procs, err := findProcAll(packageName)
		for _, proc := range procs {
			cmdline, _ := proc.CmdLine()
			suffix := "_" + strconv.Itoa(proc.PID)

			for socketPath := range unixPaths {
				if strings.HasSuffix(socketPath, suffix) ||
					(packageName == "com.android.browser" && socketPath == "chrome_devtools_remote") {
					result = append(result, map[string]interface{}{
						"pid":        proc.PID,
						"name":       cmdline[0],
						"socketPath": socketPath,
					})
				}
			}
		}
		renderJSON(w, result)
	})

	m.HandleFunc("/pidof/{pkgname}", func(w http.ResponseWriter, r *http.Request) {
		pkgname := mux.Vars(r)["pkgname"]
		pid, err := pidOf(pkgname)
		if err != nil {
			http.Error(w, err.Error(), http.StatusGone)
			return
		}
		io.WriteString(w, strconv.Itoa(pid))
	})

	m.HandleFunc("/session/{pkgname}", func(w http.ResponseWriter, r *http.Request) {
		packageName := mux.Vars(r)["pkgname"]
		mainActivity, err := mainActivityOf(packageName)
		if err != nil {
			http.Error(w, err.Error(), http.StatusGone) // 410
			return
		}
		// Refs: https://stackoverflow.com/questions/12131555/leading-dot-in-androidname-really-required
		// MainActivity convert to .MainActivity
		// com.example.app.MainActivity keep same
		// app.MainActivity keep same
		// So only words not contains dot, need to add prefix "."
		if !strings.Contains(mainActivity, ".") {
			mainActivity = "." + mainActivity
		}

		flags := r.FormValue("flags")
		if flags == "" {
			flags = "-W -S" // W: wait launched, S: stop before started
		}
		timeout := r.FormValue("timeout") // supported value: 60s, 1m. 60 is invalid
		duration, err := time.ParseDuration(timeout)
		if err != nil {
			duration = 60 * time.Second
		}

		output, err := runShellTimeout(duration, "am", "start", flags, "-n", packageName+"/"+mainActivity)
		if err != nil {
			renderJSON(w, map[string]interface{}{
				"success":      false,
				"error":        err.Error(),
				"output":       string(output),
				"mainActivity": mainActivity,
			})
		} else {
			renderJSON(w, map[string]interface{}{
				"success":      true,
				"mainActivity": mainActivity,
				"output":       string(output),
			})
		}
	}).Methods("POST")

	m.HandleFunc("/session/{pid:[0-9]+}:{pkgname}/{url:ping|jsonrpc/0}", func(w http.ResponseWriter, r *http.Request) {
		pkgname := mux.Vars(r)["pkgname"]
		pid, _ := strconv.Atoi(mux.Vars(r)["pid"])

		proc, err := procfs.NewProc(pid)
		if err != nil {
			http.Error(w, err.Error(), http.StatusGone) // 410
			return
		}
		cmdline, _ := proc.CmdLine()
		if len(cmdline) != 1 || cmdline[0] != pkgname {
			http.Error(w, fmt.Sprintf("cmdline expect [%s] but got %v", pkgname, cmdline), http.StatusGone)
			return
		}
		r.URL.Path = "/" + mux.Vars(r)["url"]
		uiautomatorProxy.ServeHTTP(w, r)
	})

	m.HandleFunc("/shell", func(w http.ResponseWriter, r *http.Request) {
		command := r.FormValue("command")
		if command == "" {
			command = r.FormValue("c")
		}
		timeoutSeconds := r.FormValue("timeout")
		if timeoutSeconds == "" {
			timeoutSeconds = "60"
		}
		seconds, err := strconv.Atoi(timeoutSeconds)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		c := Command{
			Args:    []string{command},
			Shell:   true,
			Timeout: time.Duration(seconds) * time.Second,
		}
		output, err := c.CombinedOutput()
		exitCode := cmdError2Code(err)
		renderJSON(w, map[string]interface{}{
			"output":   string(output),
			"exitCode": exitCode,
			"error":    err,
		})
	}).Methods("GET", "POST")

	m.HandleFunc("/stop", func(w http.ResponseWriter, r *http.Request) {
		log.Println("stop all service")
		service.StopAll()
		log.Println("service stopped")
		io.WriteString(w, "Finished!")
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel() // The document says need to call cancel(), but I donot known why.
			server.httpServer.Shutdown(ctx)
		}()
	})

	m.HandleFunc("/services/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := mux.Vars(r)["name"]
		var resp map[string]interface{}
		if !service.Exists(name) {
			w.WriteHeader(400) // bad request
			renderJSON(w, map[string]interface{}{
				"success":     false,
				"description": fmt.Sprintf("service %s does not exist", strconv.Quote(name)),
			})
			return
		}
		switch r.Method {
		case "GET":
			resp = map[string]interface{}{
				"success": true,
				"running": service.Running(name),
			}
		case "POST":
			err := service.Start(name)
			switch err {
			case nil:
				resp = map[string]interface{}{
					"success":     true,
					"description": "successfully started",
				}
			case cmdctrl.ErrAlreadyRunning:
				resp = map[string]interface{}{
					"success":     true,
					"description": "already started",
				}
			default:
				resp = map[string]interface{}{
					"success":     false,
					"description": "failure on start: " + err.Error(),
				}
			}
		case "DELETE":
			err := service.Stop(name)
			switch err {
			case nil:
				resp = map[string]interface{}{
					"success":     true,
					"description": "successfully stopped",
				}
			case cmdctrl.ErrAlreadyStopped:
				resp = map[string]interface{}{
					"success":     true,
					"description": "already stopped",
				}
			default:
				resp = map[string]interface{}{
					"success":     false,
					"description": "failure on stop: " + err.Error(),
				}
			}
		default:
			resp = map[string]interface{}{
				"success":     false,
				"description": "invalid request method: " + r.Method,
			}
		}
		if ok, success := resp["success"].(bool); ok {
			if !success {
				w.WriteHeader(400) // bad request
			}
		}
		renderJSON(w, resp)
	}).Methods("GET", "POST", "DELETE")

	m.HandleFunc("/raw/{filepath:.*}", func(w http.ResponseWriter, r *http.Request) {
		filepath := "/" + mux.Vars(r)["filepath"]
		http.ServeFile(w, r, filepath)
	})

	m.HandleFunc("/finfo/{lpath:.*}", func(w http.ResponseWriter, r *http.Request) {
		lpath := "/" + mux.Vars(r)["lpath"]
		finfo, err := os.Stat(lpath)
		if err != nil {
			if os.IsNotExist(err) {
				http.Error(w, err.Error(), 404)
			} else {
				http.Error(w, err.Error(), 403) // forbidden
			}
			return
		}
		data := make(map[string]interface{}, 5)
		data["name"] = finfo.Name()
		data["path"] = lpath
		data["isDirectory"] = finfo.IsDir()
		data["size"] = finfo.Size()

		if finfo.IsDir() {
			files, err := ioutil.ReadDir(lpath)
			if err == nil {
				finfos := make([]map[string]interface{}, 0, 3)
				for _, f := range files {
					finfos = append(finfos, map[string]interface{}{
						"name":        f.Name(),
						"path":        filepath.Join(lpath, f.Name()),
						"isDirectory": f.IsDir(),
					})
				}
				data["files"] = finfos
			}
		}
		renderJSON(w, data)
	})

	// keep ApkService always running
	// if no activity in 5min, then restart apk service
	const apkServiceTimeout = 5 * time.Minute
	apkServiceTimer := NewSafeTimer(apkServiceTimeout)
	go func() {
		for range apkServiceTimer.C {
			log.Println("startservice com.github.uiautomator/.Service")
			runShell("am", "startservice", "-n", "com.github.uiautomator/.Service")
			apkServiceTimer.Reset(apkServiceTimeout)
		}
	}()

	deviceInfo := getDeviceInfo()

	m.HandleFunc("/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(deviceInfo)
	})

	m.HandleFunc("/info/battery", func(w http.ResponseWriter, r *http.Request) {
		apkServiceTimer.Reset(apkServiceTimeout)
		deviceInfo.Battery.Update()
		// if err := server.tunnel.UpdateInfo(deviceInfo); err != nil {
		// 	io.WriteString(w, "Failure "+err.Error())
		// 	return
		// }
		io.WriteString(w, "Success")
	}).Methods("POST")

	m.HandleFunc("/info/rotation", func(w http.ResponseWriter, r *http.Request) {
		apkServiceTimer.Reset(apkServiceTimeout)
		var direction int                                 // 0,1,2,3
		err := json.NewDecoder(r.Body).Decode(&direction) // TODO: auto get rotation
		if err == nil {
			deviceRotation = direction * 90
			log.Println("rotation change received:", deviceRotation)
		} else {
			rotation, er := androidutils.Rotation()
			if er != nil {
				log.Println("rotation auto get err:", er)
				http.Error(w, "Failure", 500)
				return
			}
			deviceRotation = rotation
		}

		rotationPublisher.Submit(deviceRotation)

		// APK Service will send rotation to atx-agent when rotation changes
		runShellTimeout(5*time.Second, "am", "startservice", "--user", "0", "-n", "com.github.uiautomator/.Service")
		renderJSON(w, map[string]int{
			"rotation": deviceRotation,
		})
		// fmt.Fprintf(w, "rotation change to %d", deviceRotation)
	})

	/*
	 # URLRules:
	 #   URLPath ends with / means directory, eg: $DEVICE_URL/upload/sdcard/
	 #   The rest means file, eg: $DEVICE_URL/upload/sdcard/a.txt
	 #
	 # Upload a file to destination
	 $ curl -X POST -F file=@file.txt -F mode=0755 $DEVICE_URL/upload/sdcard/a.txt

	 # Upload a directory (file must be zip), URLPath must ends with /
	 $ curl -X POST -F file=@dir.zip -F dir=true $DEVICE_URL/upload/sdcard/atx-stuffs/
	*/
	m.HandleFunc("/upload/{target:.*}", func(w http.ResponseWriter, r *http.Request) {
		target := mux.Vars(r)["target"]
		if runtime.GOOS != "windows" {
			target = "/" + target
		}
		isDir := r.FormValue("dir") == "true"
		var fileMode os.FileMode
		if _, err := fmt.Sscanf(r.FormValue("mode"), "%o", &fileMode); !isDir && err != nil {
			log.Printf("invalid file mode: %s", r.FormValue("mode"))
			fileMode = 0644
		} // %o base 8

		file, header, err := r.FormFile("file")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer func() {
			file.Close()
			r.MultipartForm.RemoveAll()
		}()

		var targetDir = target
		if !isDir {
			if strings.HasSuffix(target, "/") {
				target = path.Join(target, header.Filename)
			}
			targetDir = filepath.Dir(target)
		} else {
			if !strings.HasSuffix(target, "/") {
				http.Error(w, "URLPath must endswith / if upload a directory", 400)
				return
			}
		}
		if _, err := os.Stat(targetDir); os.IsNotExist(err) {
			os.MkdirAll(targetDir, 0755)
		}

		if isDir {
			err = archiver.Zip.Read(file, target)
		} else {
			err = copyToFile(file, target)
		}

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !isDir && fileMode != 0 {
			os.Chmod(target, fileMode)
		}
		if fileInfo, err := os.Stat(target); err == nil {
			fileMode = fileInfo.Mode()
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"target": target,
			"isDir":  isDir,
			"mode":   fmt.Sprintf("0%o", fileMode),
		})
	})

	m.HandleFunc("/packages", func(w http.ResponseWriter, r *http.Request) {
		pkgs, err := listPackages()
		if err != nil {
			w.WriteHeader(500)
			renderJSON(w, map[string]interface{}{
				"success":     false,
				"description": err.Error(),
			})
			return
		}
		renderJSON(w, pkgs)
	}).Methods("GET")

	m.HandleFunc("/packages/{pkgname}/info", func(w http.ResponseWriter, r *http.Request) {
		pkgname := mux.Vars(r)["pkgname"]
		info, err := readPackageInfo(pkgname)
		if err != nil {
			renderJSON(w, map[string]interface{}{
				"success":     false,
				"description": err.Error(), // "package " + strconv.Quote(pkgname) + " not found",
			})
			return
		}
		renderJSON(w, map[string]interface{}{
			"success": true,
			"data":    info,
		})
	})

	screenshotIndex := -1
	nextScreenshotFilename := func() string {
		targetFolder := "/data/local/tmp/minicap-images"
		if _, err := os.Stat(targetFolder); err != nil {
			os.MkdirAll(targetFolder, 0755)
		}
		screenshotIndex = (screenshotIndex + 1) % 5
		return filepath.Join(targetFolder, fmt.Sprintf("%d.jpg", screenshotIndex))
	}

	m.HandleFunc("/screenshot", func(w http.ResponseWriter, r *http.Request) {
		targetURL := "/screenshot/0"
		if r.URL.RawQuery != "" {
			targetURL += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, targetURL, 302)
	}).Methods("GET")

	m.Handle("/jsonrpc/0", uiautomatorProxy)
	m.Handle("/ping", uiautomatorProxy)
	m.HandleFunc("/screenshot/0", func(w http.ResponseWriter, r *http.Request) {
		download := r.FormValue("download")
		if download != "" {
			w.Header().Set("Content-Disposition", "attachment; filename="+download)
		}

		filename := nextScreenshotFilename()

		// android emulator use screencap
		// then minicap when binary and .so exists
		// then uiautomator when service(uiautomator) is running
		// last screencap

		method := "screencap"
		if getCachedProperty("ro.product.cpu.abi") == "x86" { // android emulator
			method = "screencap"
		} else if service.Running("uiautomator") {
			method = "uiautomator"
		}

		var err error
		switch method {
		case "screencap":
			err = screenshotWithScreencap(filename)
		case "uiautomator":
			uiautomatorProxy.ServeHTTP(w, r)
			return
		}
		if err != nil && method != "screencap" {
			method = "screencap"
			err = screenshotWithScreencap(filename)
		}
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("X-Screenshot-Method", method)
		http.ServeFile(w, r, filename)
	})

	m.HandleFunc("/wlan/ip", func(w http.ResponseWriter, r *http.Request) {
		itf, err := net.InterfaceByName("wlan0")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		addrs, err := itf.Addrs()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for _, addr := range addrs {
			if v, ok := addr.(*net.IPNet); ok {
				io.WriteString(w, v.IP.String())
			}
			return
		}
		http.Error(w, "wlan0 have no ip address", 500)
	})

	var handler = cors.New(cors.Options{
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE"},
	}).Handler(m)
	// logHandler := handlers.LoggingHandler(os.Stdout, handler)
	server.httpServer = &http.Server{Handler: handler} // url(/stop) need it.
}

func (s *Server) Serve(lis net.Listener) error {
	return s.httpServer.Serve(lis)
}
