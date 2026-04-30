package sources

import "time"

const maxCooldown = 60 * time.Minute

func CooldownFor(consecutiveFailures int) time.Duration {
	if consecutiveFailures <= 0 {
		return 0
	}
	d := time.Duration(1<<(consecutiveFailures-1)) * time.Minute
	if d > maxCooldown || d < 0 { // d<0 guards against int overflow at very high counts
		return maxCooldown
	}
	return d
}
