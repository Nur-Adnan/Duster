package sysinfo

// hottestCelsius converts raw thermal zone readings to the hottest plausible
// temperature in °C, or 0 when no reading is plausible.
//
// ponytail: ACPI thermal zones track the CPU package area as the firmware sees
// it, not the die sensor. Die temperature needs a kernel driver for MSR access,
// which this tool deliberately does not load.
func hottestCelsius(raw []float64) float64 {
	var hottest float64
	for _, v := range raw {
		kelvin := v
		// "High Precision Temperature" reports tenths of Kelvin, "Temperature"
		// whole Kelvin; their plausible ranges (233-423 vs 2330-4230) never overlap.
		if v >= 1000 {
			kelvin = v / 10
		}
		// Unpopulated zones report 0 K; some firmware reports garbage like 6553.5.
		if c := kelvin - 273.15; c > 0 && c < 150 && c > hottest {
			hottest = c
		}
	}
	return hottest
}
