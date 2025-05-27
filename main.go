package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"time"
	"slices"
	"maps"

	"github.com/narqo/go-dogstatsd-parser"
)


func sortedKeys(m *map[string]any) []string {
	keys := make([]string, len(*m))
	i := 0
	for k := range *m {
		keys[i] = k
		i++
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func sortedKeysNew(m *map[string]any) []string {
	return slices.Sorted(maps.Keys(*m))
}

type MetricsMap struct {
	countMap map[string]*CountMetricSummary
	gaugeMap map[string]*GaugeMetricSummary
	timingMap map[string]*GaugeMetricSummary
	startTime time.Time
	nameMaxLen int
	tagsMaxLen int
}

func newMetricsMap(startTime time.Time) *MetricsMap {
	mm := new(MetricsMap)
	mm.startTime = startTime
	mm.nameMaxLen = 0
	mm.tagsMaxLen = 0
	mm.countMap = make(map[string]*CountMetricSummary)
	mm.gaugeMap = make(map[string]*GaugeMetricSummary)
	mm.timingMap = make(map[string]*GaugeMetricSummary)
	return mm
}

func (mm MetricsMap) IsEmpty() bool {
	return len(mm.countMap) == 0 && len(mm.gaugeMap) == 0 && len(mm.timingMap) == 0
}

func (mm MetricsMap) Print() {

	if len(mm.countMap) > 0 {
		for _, name := range slices.Sorted(maps.Keys(mm.countMap)) {
			mm.PrintOneMetric(mm.countMap[name])
		}
	}
	if len(mm.gaugeMap) > 0 {
		for _, name := range slices.Sorted(maps.Keys(mm.gaugeMap)) {
			mm.PrintOneMetric(mm.gaugeMap[name])
		}
	}
	if len(mm.timingMap) > 0 {
		for _, name := range slices.Sorted(maps.Keys(mm.timingMap)) {
			mm.PrintOneMetric(mm.timingMap[name])
		}
	}
}


func (mm *MetricsMap) AddMetric(metric *dogstatsd.Metric) {

	key := metric.Name
	switch metric.Type {
	case dogstatsd.Gauge:
		existingEntry, ok := mm.gaugeMap[key]
		if ok {
			existingEntry.AddValue(metric)
		} else {
			mm.gaugeMap[key] = newGaugeMetricSummary(metric)
		}
	case dogstatsd.Timer, "ts":
		existingEntry, ok := mm.timingMap[key]
		if ok {
			existingEntry.AddValue(metric)
		} else {
			mm.timingMap[key] = newGaugeMetricSummary(metric)
		}
	case dogstatsd.Counter:
		existingEntry, ok := mm.countMap[key]
		if ok {
			existingEntry.AddValue(metric)
		} else {
			mm.countMap[key] = newCountMetricSummary(metric)
		}
	default:
		fmt.Printf("ignoring metric of type %s\n", metric.Type)
		return
	}
}

func (mm *MetricsMap) PrintOneMetric(pms PrintableMetricSummary) {
	data := pms.GetMetric()

	// Update widths
	if len(data.Name) > mm.nameMaxLen {
		mm.nameMaxLen = len(data.Name)
	}

	// TODO: Stringify tags uniformly
	tags := tagString(data.Tags)

	if len(tags) > mm.tagsMaxLen {
		mm.tagsMaxLen = len(tags)
	}

	fmtString := fmt.Sprintf("%%s\t%%-%ds\t%%.2f\t%%-%ds\t", mm.nameMaxLen, mm.tagsMaxLen)
	fmt.Printf(fmtString, data.Type, data.Name, data.Rate, tags)

	pms.PrintSummaryValues(time.Since(mm.startTime))
	fmt.Printf("\n")
}


type PrintableMetricSummary interface {
	PrintSummaryValues(time.Duration)
	GetMetric() *dogstatsd.Metric
	AddValue(*dogstatsd.Metric)
}

// class that records and summarizes all the values of a particular metric in a time period
type CountMetricSummary struct{
	m *dogstatsd.Metric
	Sum int64
}

func newCountMetricSummary(m *dogstatsd.Metric) *CountMetricSummary {
	cms := new(CountMetricSummary)
	cms.m = m
	cms.Sum = 0

	return cms
}

func (cms *CountMetricSummary) AddValue(m *dogstatsd.Metric) {
	cms.Sum += m.Value.(int64)
}

func (cms *CountMetricSummary) GetMetric() *dogstatsd.Metric {
	return cms.m
}

func (cms *CountMetricSummary) PrintSummaryValues(d time.Duration) {
	fmt.Printf("%d\t%04f/s", cms.Sum, float64(cms.Sum) / d.Seconds())
}

type GaugeMetricSummary struct{
	m *dogstatsd.Metric
	Sum float64
	Count int64
	Last float64
}

func newGaugeMetricSummary(m *dogstatsd.Metric) *GaugeMetricSummary {
	gms := new(GaugeMetricSummary)

	gms.m = m

	gms.Sum = 0
	gms.Count = 0
	gms.Last = 0

	return gms
}

func (gms *GaugeMetricSummary) GetMetric() *dogstatsd.Metric {
	return gms.m
}

func (gms *GaugeMetricSummary) AddValue(m *dogstatsd.Metric) {
	gms.Sum += m.Value.(float64)
	gms.Count++
	gms.Last = m.Value.(float64)
}

func (gms *GaugeMetricSummary) PrintSummaryValues(d time.Duration) {
	fmt.Printf("%.4f (last)\t%.4f (avg)", gms.Last, gms.Sum / float64(gms.Count) )
}

// Adapted from https://stackoverflow.com/q/43947363
func stdoutIsTerminal() bool {
	fi, err := os.Stdout.Stat()

	if err != nil {
		return false
	}

	if (fi.Mode() & os.ModeCharDevice) == 0 {
		return false
	}

	return true
}

func tagString(tags map[string]string) string {
	keys := make([]string, 0, len(tags))

	// Copy over names and sort them
	for key, value := range tags {
		keys = append(keys, key+":"+value)
	}

	sort.Strings(keys)

	return strings.Join(keys, ",")
}

// func printMetric (data *dogstatsd.Metric) string {
// 	// Update widths
// 	if len(data.Name) > widestNameSeen {
// 		widestNameSeen = len(data.Name)
// 	}

// 	// TODO: Stringify tags uniformly
// 	tags := printTags(data.Tags)

// 	if len(tags) > widestTagsSeen {
// 		widestTagsSeen = len(tags)
// 	}

// 	fmtString := fmt.Sprintf("%%-%ds\t%%s\t%%.2f\t%%-%ds\t", widestNameSeen, widestTagsSeen)

// 	switch data.Value.(type) {
// 	case float32, float64:
// 		fmtString += "%0.2f\n"
// 	case int, int32, int64:
// 		fmtString += "%d\n"
// 	default:
// 		fmtString += "%v\n"
// 	}

// 	// TODO: Drop in a timestamp...
// 	fmt.Printf(fmtString, data.Name, data.Type, data.Rate, tags, data.Value)
// }

// func printMetricForCharDevice(data *dogstatsd.Metric) {
// 	tags := printTags(data.Tags)

// 	switch data.Value.(type) {
// 	case float32, float64:
// 		fmt.Printf("%s\t%s\t%.2f\t%s\t%0.2f\n", data.Name, data.Type, data.Rate, tags, data.Value)
// 	case int, int32, int64:
// 		fmt.Printf("%s\t%s\t%.2f\t%s\t%d\n", data.Name, data.Type, data.Rate, tags, data.Value)
// 	default:
// 		fmt.Printf("%s\t%s\t%.2f\t%s\t%v\n", data.Name, data.Type, data.Rate, tags, data.Value)
// 	}
// }

func getMetricsFromUDP(host string, port int, metricChan chan<- *dogstatsd.Metric) {
	ln, err := net.ListenUDP("udp", &net.UDPAddr{
		Port: port,
		IP:   net.ParseIP(host),
	})

	if err != nil {
		panic(err)
	}

	data := make([]byte, 65535)
	for {
		length, _, err := ln.ReadFromUDP(data)

		//fmt.Println(length, addr, err, string(data[0:length]))

		lines := strings.Fields(string(data[0:length]))
		metrics := make([]*dogstatsd.Metric, len(lines))

		//fmt.Printf("data: %+v\n", lines)

		for i, line := range lines {
			metrics[i], err = dogstatsd.Parse(line)
			if err != nil {
				fmt.Printf("ERR: %s -> %+v\n", line, err)
			} else {
				metricChan <- metrics[i]
			}
		}
	}
}

// func addMetricToMap(metricMap map[string]MetricSummary, metric *dogstatsd.Metric) {
// 	key := fmt.Sprintf("%s-%s", metric.Name, metric.Type)
// 	existingEntry, ok := metricMap[key]
// 	if ok {
// 		existingEntry.AddValue(metric)
// 	} else {
// 		switch metric.Type {
// 		case dogstatsd.Gauge, dogstatsd.Timer, "ts":
// 			metricMap[key] = newGaugeMetricSummary(metric)
// 		case dogstatsd.Counter:
// 			metricMap[key] = newCountMetricSummary(metric, )
// 		default:
// 			fmt.Printf("ignoring metric of type %s\n", metric.Type)
// 		}
// 	}
// }




func main() {
	port := flag.Int("port", 8125, "the port number to listen on")
	host := flag.String("host", "127.0.0.1", "the hostname to listen on")
	displayInterval := flag.Uint("interval", 30, "the interval in seconds at which metrics should be displayed")
	flag.Parse()

	if *displayInterval == 0 {
		*displayInterval = 30
	}

	fmt.Fprintf(os.Stderr, "listening on %s:%d and dumping every %ds\n", *host, *port, *displayInterval)

	metricChan := make(chan *dogstatsd.Metric)
	tickerChan := make(<-chan time.Time)
	displayDuration := time.Duration(*displayInterval)
	ticker := time.NewTicker(time.Second * displayDuration)
	tickerChan = ticker.C

	metricMapInterval := newMetricsMap(time.Now())
	metricMapTotal := newMetricsMap(time.Now())

	go getMetricsFromUDP(*host, *port, metricChan)

	for {
		select {
		case m := <-metricChan:
			metricMapInterval.AddMetric(m)
			metricMapTotal.AddMetric(m)

		case t := <-tickerChan:
			//don't bother printing anything if nothing was produced in the last displayInterval
			if metricMapInterval.IsEmpty() {
				continue
			}
			fmt.Println("\n\nDumping Metrics at", t)
			fmt.Printf("\nLast %d seconds\n", *displayInterval)
			metricMapInterval.Print()

			fmt.Printf("\nLast %d seconds\n", int64(time.Since(metricMapTotal.startTime).Seconds()))
			metricMapTotal.Print()

			//reset the metric Map for this interval
			metricMapInterval = newMetricsMap(time.Now())
		}
	}

}
