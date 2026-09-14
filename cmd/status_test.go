package cmd

import (
	"strings"
	"testing"

	"github.com/Nur-Adnan/duster/lib/sysinfo"
)

func TestThermalIndicator(t *testing.T) {
	tests := []struct {
		celsius float64
		want    string
	}{
		{0, "N/A"},
		{-5, "N/A"},
		{45.6, "45°C Normal"},
		{69.9, "69°C Normal"},
		{70, "70°C Warm"},
		{84.9, "84°C Warm"},
		{85, "85°C Hot"},
		{101, "101°C Hot"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := sgr.ReplaceAllString(thermalIndicator(tt.celsius), ""); got != tt.want {
				t.Errorf("thermalIndicator(%v) = %q, want %q", tt.celsius, got, tt.want)
			}
		})
	}
}

func TestStatusViewShowsTemperature(t *testing.T) {
	tests := []struct {
		celsius float64
		want    string
	}{
		{72, "Temp 72°C Warm"},
		{0, "Temp N/A"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			m := statusModel{hasStats: true, stats: sysinfo.SystemStats{CPUTempC: tt.celsius}}
			if v := sgr.ReplaceAllString(m.View(), ""); !strings.Contains(v, tt.want) {
				t.Errorf("status view missing %q", tt.want)
			}
		})
	}
}
