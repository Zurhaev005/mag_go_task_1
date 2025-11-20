package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	urlStats = "http://srv.msk01.gigacorp.local/_stats"

	loadAvgLimit           = 30.0
	memUsageLimit          = 0.80
	diskUsageLimit         = 0.90
	netUsageLimit          = 0.90
	errorThreshold         = 3
	pollInterval           = 2 * time.Second 
	bytesInMB        int64 = 1024 * 1024
	bitsInMbit       int64 = 1024 * 1024
)

func main() {
	ctx := context.Background()
	if err := run(ctx, os.Stdout, urlStats, pollInterval); err != nil {

	}
}

func run(ctx context.Context, out io.Writer, endpoint string, interval time.Duration) error {
	client := &http.Client{Timeout: 5 * time.Second}
	errCount := 0

	t := time.NewTicker(interval)
	defer t.Stop()

	for {
		if err := pollOnce(ctx, client, endpoint, out); err != nil {
			errCount++
			if errCount >= errorThreshold {
				fmt.Fprintln(out, "Unable to fetch server statistic")
			}
		} else {
			// сбрасываем счётчик ошибок
			errCount = 0
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

func pollOnce(ctx context.Context, client *http.Client, endpoint string, out io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	body, err := readAll(resp.Body)
	if err != nil {
		return err
	}

	fields := strings.Split(strings.TrimSpace(body), ",")
	if len(fields) != 7 {
		return fmt.Errorf("bad fields: %d", len(fields))
	}

	// парсим значения
	toF := func(s string) (float64, error) {
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	}
	toI := func(s string) (int64, error) {
		v, e := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		return v, e
	}

	loadAvg, err := toF(fields[0])
	if err != nil {
		return err
	}
	memTotal, err := toI(fields[1])
	if err != nil {
		return err
	}
	memUsed, err := toI(fields[2])
	if err != nil {
		return err
	}
	diskTotal, err := toI(fields[3])
	if err != nil {
		return err
	}
	diskUsed, err := toI(fields[4])
	if err != nil {
		return err
	}
	netTotalBps, err := toI(fields[5])
	if err != nil {
		return err
	}
	netUsedBps, err := toI(fields[6])
	if err != nil {
		return err
	}

	// условия
	if loadAvg > loadAvgLimit {
		fmt.Fprintf(out, "Load Average is too high: %.0f\n", loadAvg)
	}

	if memTotal > 0 {
		memPct := float64(memUsed) / float64(memTotal) * 100
		if memPct > memUsageLimit*100 {
			fmt.Fprintf(out, "Memory usage too high: %.0f%%\n", memPct)
		}
	}

	if diskTotal > 0 {
		usedPct := float64(diskUsed) / float64(diskTotal)
		if usedPct > diskUsageLimit {
			freeBytes := diskTotal - diskUsed
			freeMb := freeBytes / bytesInMB
			fmt.Fprintf(out, "Free disk space is too low: %d Mb left\n", freeMb)
		}
	}

	if netTotalBps > 0 {
		usedPct := float64(netUsedBps) / float64(netTotalBps)
		if usedPct > netUsageLimit {
			freeBps := netTotalBps - netUsedBps
			// переведём байты/с в мегабиты/с: (bytes * 8) / 1024 / 1024
			freeMbit := (freeBps * 8) / bitsInMbit
			fmt.Fprintf(out, "Network bandwidth usage high: %d Mbit/s available\n", freeMbit)
		}
	}

	return nil
}

func readAll(r io.Reader) (string, error) {
	var b strings.Builder
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		b.WriteString(sc.Text())
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return b.String(), nil
}
