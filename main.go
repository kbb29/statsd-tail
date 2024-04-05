package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/narqo/go-dogstatsd-parser"
)

var widestNameSeen int
var widestTagsSeen int

// Keep track of how our data should be displayed
// TODO: Check terminal width and space appropriately
func init() {
	widestNameSeen = 0
	widestTagsSeen = 0
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

func printTags(tags map[string]string) string {
	keys := make([]string, 0, len(tags))

	// Copy over names and sort them
	for key, value := range tags {
		keys = append(keys, key+":"+value)
	}

	sort.Strings(keys)

	return strings.Join(keys, ",")
}

func printMetricForTerminal(data *dogstatsd.Metric) {
	// Update widths
	if len(data.Name) > widestNameSeen {
		widestNameSeen = len(data.Name)
	}

	// TODO: Stringify tags uniformly
	tags := printTags(data.Tags)

	if len(tags) > widestTagsSeen {
		widestTagsSeen = len(tags)
	}

	fmtString := fmt.Sprintf("%%-%ds\t%%s\t%%.2f\t%%-%ds\t", widestNameSeen, widestTagsSeen)

	switch data.Value.(type) {
	case float32, float64:
		fmtString += "%0.2f\n"
	case int, int32, int64:
		fmtString += "%d\n"
	default:
		fmtString += "%v\n"
	}

	// TODO: Drop in a timestamp...
	fmt.Printf(fmtString, data.Name, data.Type, data.Rate, tags, data.Value)

}

func printMetricForCharDevice(data *dogstatsd.Metric) {
	tags := printTags(data.Tags)

	switch data.Value.(type) {
	case float32, float64:
		fmt.Printf("%s\t%s\t%.2f\t%s\t%0.2f\n", data.Name, data.Type, data.Rate, tags, data.Value)
	case int, int32, int64:
		fmt.Printf("%s\t%s\t%.2f\t%s\t%d\n", data.Name, data.Type, data.Rate, tags, data.Value)
	default:
		fmt.Printf("%s\t%s\t%.2f\t%s\t%v\n", data.Name, data.Type, data.Rate, tags, data.Value)
	}
}

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

func addMetricToMap(metricMap map[string]*dogstatsd.Metric, metric *dogstatsd.Metric) {
	key := fmt.Sprintf("%s-%s", metric.Name, metric.Type)
	switch metric.Type {
	case "g", "ts", "ms":
		metricMap[key] = metric
	case "c":
		existingEntry, ok := metricMap[key]
		if ok {
			existingEntry.Value = metric.Value.(int64) + existingEntry.Value.(int64)
		} else {
			metricMap[key] = metric
		}
	default:
		fmt.Printf("ignoring metric of type %s\n", metric.Type)
	}
}

func makeMetricsMap() map[string]*dogstatsd.Metric {
	return make(map[string]*dogstatsd.Metric)
}

func sortedKeys(m map[string]*dogstatsd.Metric) []string {
	keys := make([]string, len(m))
	i := 0
	for k := range m {
		keys[i] = k
		i++
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
}

func main() {
	port := flag.Int("port", 8125, "the port number to listen on")
	host := flag.String("host", "127.0.0.1", "the hostname to listen on")
	displayInterval := flag.Uint("interval", 30, "the interval in seconds at which metrics should be displayed")
	flag.Parse()

	fmt.Fprintf(os.Stderr, "listening on %s:%d\n", *host, *port)

	printer := printMetricForTerminal

	if !stdoutIsTerminal() {
		printer = printMetricForCharDevice
	}

	metricChan := make(chan *dogstatsd.Metric)
	tickerChan := make(<-chan time.Time)
	if *displayInterval > 0 {
		ticker := time.NewTicker(time.Second * time.Duration(*displayInterval))
		tickerChan = ticker.C
	}

	metricMapInterval := makeMetricsMap()
	metricMapTotal := makeMetricsMap()

	go getMetricsFromUDP(*host, *port, metricChan)

	for {
		select {
		case m := <-metricChan:
			if *displayInterval == 0 {
				printer(m)
			} else {
				addMetricToMap(metricMapInterval, m)
				addMetricToMap(metricMapTotal, m)
			}

		case t := <-tickerChan:
			//don't bother printing anything if nothing was produced in the last displayInterval
			if len(metricMapInterval) == 0 {
				continue
			}
			fmt.Println("\n\nDumping Metrics at", t)
			fmt.Printf("\nLast %d seconds\n", *displayInterval)
			for _, name := range sortedKeys(metricMapInterval) {
				printer(metricMapInterval[name])
			}
			fmt.Println("\nTotal")
			for _, name := range sortedKeys(metricMapTotal) {
				printer(metricMapTotal[name])
			}
			//reset the metric Map for this interval
			metricMapInterval = makeMetricsMap()
		}
	}

}
