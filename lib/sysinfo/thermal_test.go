package sysinfo

import (
	"math"
	"testing"
)

func TestHottestCelsius(t *testing.T) {
	tests := []struct {
		name string
		raw  []float64
		want float64
	}{
		{"no zones", nil, 0},
		{"whole kelvin", []float64{323.15}, 50},
		{"tenths of kelvin", []float64{3231.5}, 50},
		{"hottest zone wins", []float64{3031.5, 3431.5, 3231.5}, 70},
		{"unpopulated and garbage zones ignored", []float64{0, 6553.5, 3131.5}, 40},
		{"nothing plausible", []float64{0, 273.15}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hottestCelsius(tt.raw); math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("hottestCelsius(%v) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}
