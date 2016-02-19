package main

import (
	"log"
	"math"
	"syscall"
	"strings"
	"strconv"
)

func setLimit(rtype string, limit string) {
	if (limit == "") {
		return
	}
	var resource int
	switch rtype {
		case "rlimit-core":
			resource = syscall.RLIMIT_CORE
		case "rlimit-cpu":
			resource = syscall.RLIMIT_CPU
		case "rlimit-data":
			resource = syscall.RLIMIT_DATA
		case "rlimit-fsize":
			resource = syscall.RLIMIT_FSIZE
		case "rlimit-nofile":
			resource = syscall.RLIMIT_NOFILE
		case "rlimit-stack":
			resource = syscall.RLIMIT_STACK
	}
	var rlim syscall.Rlimit
	err := syscall.Getrlimit(resource, &rlim)
	if err != nil {
		log.Fatalf("Error getrlimit:%s\n", err)
	}
	if (strings.ToLower(limit) == "unlimited") {
		rlim.Cur = math.MaxUint64
		rlim.Max = math.MaxUint64
	} else {
		rlim.Cur, err = strconv.ParseUint(limit, 10, 64)
		if err != nil {
			log.Fatalf("Error invalid rlimit %s:%s\n", limit, err)
		}
	 	if rlim.Cur >= rlim.Max {
			rlim.Max = rlim.Cur
	 	}
	}
	err = syscall.Setrlimit(resource, &rlim)
	if err != nil {
		log.Fatalf("Error setrlimit:%s\n", err)
	}
}
