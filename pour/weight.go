package pour

var (
	flowRate = map[[2]int][]float64{
		{1399, 1300}: {0, 2000, 2400},
		{1299, 1200}: {-0.05, 1650, 1800},
		{1199, 1100}: {-0.06, 1700, 2050},
		{1099, 1000}: {-0.075, 2050, 2900},
		{999, 900}:   {-0.15, 1250, 1900},
		{899, 800}:   {-0.25, 1000, 3000},
	}
)

func getAngleAndSleep(bottleWeight int) []float64 {
	info := make([]float64, 2)

	// handle case where the weight is outside of the ranges defined by our map
	if bottleWeight > 1399 {
		l := flowRate[[2]int{1399, 1300}]
		info[0] = l[0]
		info[1] = l[1]
	}
	if bottleWeight < 800 {
		l := flowRate[[2]int{899, 800}]
		info[0] = l[0]
		info[1] = l[2]
	}

	for k, v := range flowRate {
		upperBound := k[0]
		lowerBound := k[1]
		if upperBound >= bottleWeight && bottleWeight >= lowerBound {
			info[0] = v[0]
			if bottleWeight == upperBound {
				info[1] = v[1]
				break
			}
			if bottleWeight == lowerBound {
				info[1] = v[2]
				break
			}
			weightDiff := bottleWeight - lowerBound
			percentChange := float64(weightDiff) / float64((upperBound - lowerBound)) // upperBound - lowerBound always equals 99
			timeDiff := v[2] - v[1]
			timeIncrement := percentChange * timeDiff
			timeToPour := v[1] + float64(timeIncrement)
			info[1] = timeToPour
		}
	}
	return info
}
