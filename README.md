#atx-agent
[![Build Status](https://travis-ci.org/openatx/atx-agent.svg?branch=master)](https://travis-ci.org/openatx/atx-agent)

The main purpose of this project is to shield the differences between different Android machines, and then open a unified HTTP interface for [openatx/uiautomator2](https://github.com/openatx/uiautomator2) to use. The project will eventually be released as a binary program running in the background of the Android system.

How does this project shield the differences between different machines? For example, the operation of taking a screenshot requires about 3 judgments.

1. First determine whether the minicap is installed and available, and then take a screenshot of the minicap. After all, minicap has the fastest screenshot speed
2. Use the interface screenshot provided by uiautomator2. (except simulator)
3. Use screencap to take a screenshot, and then adjust the rotation direction according to the rotation of the screen. (Generally only the emulator takes screenshots in this way)

It is the different manifestations of Android phones that lead to the need for so many judgments. And atx-agent is to help you deal with these operations. Then provide a unified HTTP interface (GET /screenshot) for your use.

#Develop
This project is written in Go language. When compiling, you need to have a little foundation of Go language.
For more content, see [DEVELOP.md](DEVELOP.md)

# Installation
Download the binary package ending with `linux_armv7.tar.gz` from <https://github.com/openatx/atx-agent/releases>. Most mobile phones are based on linux-arm architecture.

Unzip the `atx-agent` file and open the console
```bash
$ adb push atx-agent /data/local/tmp
$ adb shell chmod 755 /data/local/tmp/atx-agent
# launch atx-agent in daemon mode
$ adb shell /data/local/tmp/atx-agent server -d

# stop already running atx-agent and start daemon
$ adb shell /data/local/tmp/atx-agent server -d --stop
```

The default listening port is 7912.

# common interface
Suppose the address of the mobile phone is $DEVICE_URL (eg: `http://10.0.0.1:7912`)

## Get phone screenshots
```bash
# jpeg format image
$ curl $DEVICE_URL/screenshot

# Use the built-in uiautomator to take screenshots
$ curl "$DEVICE_URL/screenshot/0?minicap=false"
```

## Get the current program version
```bash
$ curl $DEVICE_URL/version
# expect example: 0.0.2
```

## Get device information
```bash
$ curl $DEVICE_URL/info
{
     "udid": "bf755cab-ff:ff:ff:ff:ff:ff-SM901",
     "serial": "bf755cab",
     "brand": "SMARTISAN",
     "model": "SM901",
     "hwaddr": "ff:ff:ff:ff:ff:ff",
     "agentVersion": "dev"
}
```

## Get Hierarchy
This interface is currently relatively advanced. When communication with uiautomator fails, it will start the uiautomator service in the background, and return data when it returns to normal.

```bash
$ curl $DEVICE_URL /dump/hierarchy
{
     "jsonrpc": "2.0",
     "id": 1559113464,
     "result": "<?xml version='1.0> ... hierarchy ..."
}

# Stop uiautomator
$ curl -X DELETE $DEVICE_URL/uiautomator
Success

# Call again, still OK, but may have to wait for 7~8s
$ curl $DEVICE_URL /dump/hierarchy
{
     "jsonrpc": "2.0",
     ...
}
```

## Install the app
```bash
$ curl -X POST -d url="http://some-host/some.apk" $DEVICE_URL/install
# expect install id
2
# get install progress
$ curl -X GET $DEVICE_URL/install/1
{
     "id": "2",
     "titalSize": 770571,
     "copiedSize": 770571,
     "message": "success installed"
}
```

## Shell commands
```bash
$ curl -X POST -d command="pwd" $DEVICE_URL/shell
{
     "output": "/",
     "error": null
}
```

Background Shell command (can run continuously in the background and will not be killed)

```bash
$ curl -X POST -d command="pwd" $DEVICE_URL/shell/background
{
     "success": true,
}
```

## Webview related
```bash
$ curl -X GET $DEVICE_URL/webviews
[
     "webview_devtools_remote_m6x_21074",
     "webview_devtools_remote_m6x_27681",
     "chrome_devtools_remote"
]
```


## App information acquisition
```bash
# Get all running applications
$ curl $DEVICE_URL /proc/list
[
     {
         "cmdline": ["/system/bin/adbd", "--root_seclabel=u:r:su:s0"],
         "name": "adbd",
         "pid": 16177
     },
     {
         "cmdline": ["com.netease.cloudmusic"],
         "name": "com.netease.cloudmusic",
         "pid": 15532
     }
]

# Get the memory information of the application (the data is for reference only), the unit is KB, and total represents the PSS of the application
$ curl $DEVICE_URL /proc/com.netease.cloudmusic/meminfo
{
     "code": 17236,
     "graphics": 20740,
     "java heap": 22288,
     "native heap": 20576,
     "private other": 10632,
     "stack": 48,
     "system": 110925,
     "total": 202445,
     "total swap pss": 88534
}

# Get the memory data of the application and all its child processes
$ curl $DEVICE_URL /proc/com.netease.cloudmusic/meminfo/all
{
     "com.netease.cloudmusic": {
         "code": 15952,
         "graphics": 19328,
         "java heap": 45488,
         "native heap": 20840,
         "private other": 4056,
         "stack": 956,
         "system": 18652,
         "total": 125272
     },
     "com.netease.cloudmusic:browser": {
         "code": 848,
         "graphics": 12,
         "java heap": 6580,
         "native heap": 5428,
         "private other": 1592,
         "stack": 336,
         "system": 10603,
         "total": 25399
     }
}
```

# Get CPU information

If the process is multi-threaded and the machine is multi-core, the returned CPU Percent may be greater than 100%

```bash
curl $DEVICE_URL/proc/<package or pid>/cpuinfo
# success return
{
     "pid": 1122,
     "user": 288138,
     "system": 73457,
     "percent": 50.0,
     "systemPercent": 88.372,
     "coreCount": 4,
}
# failure return
410 Gone, Or 500 Internal error
```


## download file
```bash
$ curl $DEVICE_URL/raw/sdcard/tmp.txt
```

## upload files
```bash
# Upload to the /sdcard directory (url ends with /)
$ curl -F "file=@somefile.txt" $DEVICE_URL/upload/sdcard/

# Upload to /sdcard/tmp.txt
$ curl -F "file=@somefile.txt" $DEVICE_URL/upload/sdcard/tmp.txt
```

Upload directory (url must end with /)

```bash
$ curl -F file=@some.zip -F dir=true $DEVICE_URL/upload/sdcard/
```

## Get file and directory information
```bash
# document
$ curl -X GET $DEVICE_URL/finfo/data/local/tmp/tmp.txt
{
"name": "tmp.txt",
"path": "/data/local/tmp/tmp.txt",
"isDirectory": false,
"size": 15232,
}

# Table of contents
$ curl -X GET $DEVICE_URL /finfo/data/local/tmp
{
"name": "tmp",
"path": "/data/local/tmp",
"isDirectory": true,
"size": 8192,
"files": [
{
"name": "tmp.txt",
"path": "/data/local/tmp/tmp.txt"
"isDirectory": false,
}
]
}
```

It is equivalent to upload `some.zip` to the mobile phone, and then execute `unzip some.zip -d /sdcard`, finally delete `some.zip`

## Download offline
```bash
# Offline download, return ID
$ curl -F url=https://.... -F filepath=/sdcard/some.txt -F mode=0644 $DEVICE_URL/download
1
# Check the download status by the returned ID
$ curl $DEVICE_URL/download/1
{
     "message": "downloading",
     "progress": {
         "totalSize": 15000,
         "copiedSize": 10000
     }
}
```

## uiautomator start and stop
```bash
# start up
$ curl -X POST $DEVICE_URL/uiautomator
Success

# stop
$ curl -X DELETE $DEVICE_URL/uiautomator
Success

# stop again
$ curl -X DELETE $DEVICE_URL/uiautomator
Already stopped

# Get uiautomator status
$ curl $DEVICE/uiautomator
{
     "running": true
}
```

## Start the application
```bash
# timeout represents the timeout period of am start -n
# flags default to -S -W
$ http POST $DEVICE_URL/session/{com.cleanmaster.mguard_cn} timeout==10s flags=="-S"
{
     "mainActivity": "com.keniu.security.main.MainActivity",
     "output": "Stopping: com.cleanmaster.mguard_cn\nStarting: Intent { cmp=com.cleanmaster.mguard_cn/com.keniu.security.main.MainActivity }\n",
     "success": true
}
```

## Get package information
```bash
$ http GET $DEVICE_URL/packages/{packageName}/info
{
     "success": true,
     "data": {
         "mainActivity": "com.github.uiautomator.MainActivity",
         "label": "ATX",
         "versionName": "1.1.7",
         "versionCode": 1001007,
         "size":1760809
     }
}
```

Where `size` is in bytes

## Get the icon of the package
```
$ curl -XGET $DEVICE_URL/packages/{packageName}/icon
# Returns the package's icon file
# In case of failure status code != 200
```

## Get information about all packages
The interface speed is a bit slow, it takes about 3s.

The principle is to get the package information through `pm list packages -3 -f`, and then use the `androidbinary` library to parse the package

```bash
$ http GET $DEVICE_URL/packages
[
     {
         "packageName": "com.github.uiautomator",
         "mainActivity": "com.github.uiautomator.MainActivity",
         "label": "ATX",
         "versionName": "1.1.7-2-361182f-dirty",
         "versionCode": 1001007,
         "size": 1639366
     },
     {
         "packageName": "com.smartisanos.payment",
         "mainActivity": "",
         "label": "",
         "versionName": "1.1",
         "versionCode": 1,
         "size": 3910826
     },
     ...
]
```

## Adjust the automatic stop time of uiautomator (default 3 minutes)
```bash
$ curl -X POST 10.0.0.1:7912/newCommandTimeout --data 300
{
     "success": true,
     "description": "newCommandTimeout updated to 5m0s"
}
```

## Program self-upgrade (temporarily unavailable)
The upgrade program is directly downloaded from github releases, and it will automatically restart after the upgrade

upgrade to latest version

```bash
$ curl 10.0.0.1:7912/upgrade
```

Specify the version to upgrade

```bash
$ curl "10.0.0.1:7912/upgrade?version=0.0.2"
```

## Repair minicap, minitouch program

```bash
#Fix minicap
$ curl -XPUT 10.0.0.1:7912/minicap

# Fix minitouch
$ curl -XPUT 10.0.0.1:7912/minitouch
```

## Video recording (not recommended)
start recording

```bash
$ curl -X POST 10.0.0.1:7912/screenrecord
```

Stop recording and get recording result

```bash
$ curl -X PUT 10.0.0.1:7912/screenrecord
{
     "videos": [
         "/sdcard/screenrecords/0.mp4",
         "/sdcard/screenrecords/1.mp4"
     ]
}
```

Then download it locally

```bash
$ curl -X GET 10.0.0.1:7912/raw/sdcard/screenrecords/0.mp4
```

## Minitouch operation method
Thanks to [openstf/minitouch](https://github.com/openstf/minitouch)

Websocket connection `$DEVICE_URL/minitouch`, written line by line in JSON format

> Note: The coordinate origin is always the upper left corner when the phone is placed upright, and the user needs to handle the rotation change by himself

Please read minitouch's [Usage](https://github.com/openstf/minitouch#usage) document first, and then look at the following part

-Touch Down

     Coordinates (X: 50%, Y: 50%), index represents the number of fingers, `pressure` is optional.

     ```json
     {"operation": "d", "index": 0, "xP": 0.5, "yP": 0.5, "pressure": 50}
     ```

- Touch Commit

     ```json
     {"operation": "c"}
     ```

-Touch Move

     ```json
     {"operation": "m", "index": 0, "xP": 0.5, "yP": 0.5, "pressure": 50}
     ```

-Touch Up

     ```json
     {"operation": "u", "index": 0}
     ```

- Click on x:20%, y:20, slide to x:40%, y:50%

     ```json
     {"operation": "d", "index": 0, "xP": 0.20, "yP": 0.20, "pressure": 50}
     {"operation": "c"}
     {"operation": "m", "index": 0, "xP": 0.40, "yP": 0.50, "pressure": 50}
     {"operation": "c"}
     {"operation": "u", "index": 0}
     {"operation": "c"}
     ```

# TODO
1. At present, security is still a problem, and we will find ways to improve it in the future
2. Complete the interface document
3. Security issues of the built-in webpage adb shell

# Logs
log path `/sdcard/atx-agent.log`

##TODO
- [ ] Use a library that supports multi-threaded downloads https://github.com/cavaliercoder/grab

# LICENSE
[MIT](LICENSE)