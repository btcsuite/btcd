package main

import (
	"fmt"

	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"

	"os"
	"path/filepath"
	"time"
)

func toGB(n uint64) float64 {
	return float64(n) / 1024.0 / 1024.0 / 1024.0
}

func dirSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return err
	})
	return size, err
}

func logMemoryUsage() {
	last := ""
	tick := time.NewTicker(40 * time.Second)
	for range tick.C {
		m, err := mem.VirtualMemory()
		if err != nil {
			btcdLog.Warnf("When reading memory size: %s", err.Error())
			continue
		}

		d, err := disk.Usage(cfg.DataDir)
		if err != nil {
			btcdLog.Warnf("When reading disk usage: %s", err.Error())
			continue
		}

		p, err := process.NewProcess(int32(os.Getpid()))
		if err != nil {
			btcdLog.Warnf("When reading process: %s", err.Error())
			continue
		}

		m2, err := p.MemoryInfo()
		if err != nil {
			btcdLog.Warnf("When reading memory info: %s", err.Error())
			continue
		}

		ds, err := dirSize(cfg.DataDir)
		if err != nil {
			btcdLog.Debugf("When reading directory: %s", err.Error())
			continue
		}

		cur := fmt.Sprintf("RAM: using %.1f GB with %.1f available, DISK: using %.1f GB with %.1f available",
			toGB(m2.RSS), toGB(m.Available), toGB(uint64(ds)), toGB(d.Free))
		if cur != last {
			btcdLog.Infof(cur)
			last = cur
		}
	}
}
