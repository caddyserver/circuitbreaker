// Copyright 2015 Matthew Holt and The Caddy Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Original implementation by Danny Navarro @ Ardan Labs.

package circuitbreaker

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"github.com/vulcand/oxy/memmetrics"
)

func init() {
	caddy.RegisterModule(Simple{})
}

// Simple implements circuit breaking functionality for
// requests within this process over a sliding time window.
type Simple struct {
	tripped  int32 // accessed atomically
	cbFactor int32
	metrics  *memmetrics.RTMetrics
	Config
}

// CaddyModule returns the Caddy module information.
func (Simple) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.reverse_proxy.circuit_breakers.simple",
		New: func() caddy.Module { return new(Simple) },
	}
}

// Provision sets up a configured circuit breaker.
func (c *Simple) Provision(ctx caddy.Context) error {
	f, ok := typeCB[c.Factor]
	if !ok {
		return fmt.Errorf("type is not defined")
	}

	if c.TripDuration == 0 {
		c.TripDuration = caddy.Duration(defaultTripDuration)
	}

	mt, err := memmetrics.NewRTMetrics()
	if err != nil {
		return fmt.Errorf("cannot create new metrics: %v", err.Error())
	}

	c.cbFactor = f
	c.metrics = mt
	c.tripped = 0

	return nil
}

// OK returns whether the circuit breaker is tripped or not.
func (c *Simple) OK() bool {
	return atomic.LoadInt32(&c.tripped) == 0
}

// RecordMetric records a response status code and execution time of a request. This function should be run in a separate goroutine.
func (c *Simple) RecordMetric(statusCode int, latency time.Duration) {
	c.metrics.Record(statusCode, latency)
	c.checkAndSet()
}

// Ok checks our metrics to see if we should trip our circuit breaker, or if the fallback duration has completed.
func (c *Simple) checkAndSet() {
	var isTripped bool

	switch c.cbFactor {
	case factorErrorRatio:
		// check if amount of network errors exceed threshold over sliding window, threshold for comparison should be < 1.0 i.e. .5 = 50th percentile
		if c.metrics.NetworkErrorRatio() > c.Threshold {
			isTripped = true
		}
	case factorLatency:
		// check if threshold in milliseconds is reached and trip
		hist, err := c.metrics.LatencyHistogram()
		if err != nil {
			return
		}

		l := hist.LatencyAtQuantile(c.Threshold)
		if l.Nanoseconds()/int64(time.Millisecond) > int64(c.Threshold) {
			isTripped = true
		}
	case factorStatusCodeRatio:
		// check ratio of error status codes of sliding window, threshold for comparison should be < 1.0 i.e. .5 = 50th percentile
		if c.metrics.ResponseCodeRatio(500, 600, 0, 600) > c.Threshold {
			isTripped = true
		}
	}

	if isTripped {
		c.metrics.Reset()
		atomic.AddInt32(&c.tripped, 1)

		// wait TripDuration amount before allowing operations to resume.
		t := time.NewTimer(time.Duration(c.Config.TripDuration))
		<-t.C

		atomic.AddInt32(&c.tripped, -1)
	}
}

// Config represents the configuration of a circuit breaker.
type Config struct {
	// The threshold over sliding window that would trip the circuit breaker
	Threshold float64 `json:"threshold,omitempty"`
	// Possible values: latency, error_ratio, and status_ratio. It
	// defaults to latency.
	Factor string `json:"factor,omitempty"`
	// How long to wait after the circuit is tripped before allowing operations to resume.
	// The default is 5s.
	TripDuration caddy.Duration `json:"trip_duration,omitempty"`
}

const (
	factorLatency = iota + 1
	factorErrorRatio
	factorStatusCodeRatio
	defaultTripDuration = 5 * time.Second
)

// typeCB handles converting a Config Factor value to the internal circuit breaker types.
var typeCB = map[string]int32{
	"latency":      factorLatency,
	"error_ratio":  factorErrorRatio,
	"status_ratio": factorStatusCodeRatio,
}

// Interface guards
var (
	_ caddy.Provisioner           = (*Simple)(nil)
	_ reverseproxy.CircuitBreaker = (*Simple)(nil)
)
