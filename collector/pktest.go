// Copyright 2019 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build !nocpu

package collector

import (
	"fmt"
	"path"
	"path/filepath"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus"
)

type PKTEST struct {
	power       *prometheus.Desc
	packages    map[string]string
	prevReading map[string]float64
	prevTime    uint64
	logger      log.Logger
}

func getCurrMS() uint64 {
	return uint64(time.Now().UnixNano() / 1000000)
}

func getPackages() (map[string]string, error) {
	Packages, err := filepath.Glob("/sys/bus/node/devices/node[0-9]*")
	if err != nil {
		return nil, fmt.Errorf("failed to open sysfs: %w", err)
	}

	retMap := make(map[string]string)

	for index := 0; index < len(Packages); index++ {
		pkgName := path.Base(Packages[index])
		rplPath := "/sys/devices/virtual/powercap/intel-rapl/intel-rapl:" + pkgName[4:]
		retMap[pkgName] = rplPath
	}

	return retMap, err
}

func init() {
	registerCollector("pktest", defaultEnabled, NewPkTestCollector)
}

// NewCPUFreqCollector returns a new Collector exposing kernel/system statistics.
func NewPkTestCollector(logger log.Logger) (Collector, error) {
	Packages, err := getPackages()
	if err != nil {
		return nil, fmt.Errorf("failed to open sysfs: %w", err)
	}

	startPower := make(map[string]float64)
	/* initialize start power */
	for packageID, dataDir := range Packages {
		powerUsage, err := calcPower(dataDir)
		if err != nil {
			return nil, fmt.Errorf("failed to open sysfs: %w", err)
		}
		startPower[packageID] = powerUsage
	}
	/* when did we start */
	startTime := getCurrMS()

	return &PKTEST{
		power: prometheus.NewDesc(
			prometheus.BuildFQName(namespace, "node_power", "power_watts"),
			"Current cpu node power consumption in watts.",
			[]string{"node"}, nil,
		),
		logger:      logger,
		packages:    Packages,
		prevReading: startPower,
		prevTime:    startTime,
	}, nil

}

func calcPower(metricDir string) (float64, error) {
	value, err := readUintFromFile(filepath.Join(metricDir, "energy_uj"))
	if err != nil {
		return 0.0, err
	}

	return float64(value) / 1000000.0, nil // convert from  uJ to J
}

// Update implements Collector and exposes cpu related metrics from /proc/stat and /sys/.../cpu/.
func (collector *PKTEST) Update(ch chan<- prometheus.Metric) error {

	currTime := getCurrMS()
	prevTime := collector.prevTime
	collector.prevTime = currTime

	// how many seconds have elapsed
	timeDelta := float64((currTime - prevTime)) / 1000.0

	for packageID, dataDir := range collector.packages {
		powerUsage, err := calcPower(dataDir)
		if nil != err {
			return nil
		}
		powerDelta := powerUsage - collector.prevReading[packageID]
		collector.prevReading[packageID] = powerUsage
		watts := powerDelta / timeDelta
		ch <- prometheus.MustNewConstMetric(
			collector.power,
			prometheus.GaugeValue,
			watts,
			packageID,
		)
	}

	return nil
}
