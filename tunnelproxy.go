package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/openatx/androidutils"
)

var currentDeviceInfo *DeviceInfo

func getDeviceInfo() *DeviceInfo {
	if currentDeviceInfo != nil {
		return currentDeviceInfo
	}
	devInfo := &DeviceInfo{
		Serial:       getCachedProperty("ro.serialno"),
		Brand:        getCachedProperty("ro.product.brand"),
		Model:        getCachedProperty("ro.product.model"),
		Version:      getCachedProperty("ro.build.version.release"),
		AgentVersion: version,
	}
	devInfo.Sdk, _ = strconv.Atoi(getCachedProperty("ro.build.version.sdk"))
	devInfo.HWAddr, _ = androidutils.HWAddrWLAN()
	display, _ := androidutils.WindowSize()
	devInfo.Display = &display
	battery := androidutils.Battery{}
	battery.Update()
	devInfo.Battery = &battery
	// devInfo.Port = listenPort

	memory, err := androidutils.MemoryInfo()
	if err != nil {
		log.Println("get memory error:", err)
	} else {
		total := memory["MemTotal"]
		around := int(math.Ceil(float64(total-512*1024) / 1024.0 / 1024.0)) // around GB
		devInfo.Memory = &MemoryInfo{
			Total:  total,
			Around: fmt.Sprintf("%d GB", around),
		}
	}

	hardware, processors, err := androidutils.ProcessorInfo()
	if err != nil {
		log.Println("get cpuinfo error:", err)
	} else {
		devInfo.Cpu = &CpuInfo{
			Hardware: hardware,
			Cores:    len(processors),
		}
	}

	// Udid is ${Serial}-${MacAddress}-${model}
	udid := fmt.Sprintf("%s-%s-%s",
		getCachedProperty("ro.serialno"),
		devInfo.HWAddr,
		strings.Replace(getCachedProperty("ro.product.model"), " ", "_", -1))
	devInfo.Udid = udid
	currentDeviceInfo = devInfo
	return currentDeviceInfo
}

