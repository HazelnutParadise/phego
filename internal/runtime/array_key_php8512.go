package runtime

import "fmt"

// php8512IntegerStringKey identifies a decimal string that PHP 8.5.12 treats
// as an integer array key on a platform with the specified integer width.
// A valid string returns its value and true; other strings return false.
// Unsupported widths return an error. Callers must select the exact profile.
func php8512IntegerStringKey(s string, width int) (int64, bool, error) {
	var maxPositive, maxNegative uint64
	switch width {
	case 32:
		maxPositive = 1<<31 - 1
		maxNegative = 1 << 31
	case 64:
		maxPositive = 1<<63 - 1
		maxNegative = 1 << 63
	default:
		return 0, false, fmt.Errorf("unsupported PHP integer width %d: want 32 or 64", width)
	}

	if len(s) == 0 {
		return 0, false, nil
	}

	negative := s[0] == '-'
	digits := s
	if negative {
		digits = s[1:]
	}
	if len(digits) == 0 || (len(digits) > 1 && digits[0] == '0') {
		return 0, false, nil
	}
	if negative && digits[0] == '0' {
		return 0, false, nil
	}

	limit := maxPositive
	if negative {
		limit = maxNegative
	}
	var magnitude uint64
	for i := 0; i < len(digits); i++ {
		digit := digits[i] - '0'
		if digit > 9 {
			return 0, false, nil
		}
		if magnitude > (limit-uint64(digit))/10 {
			return 0, false, nil
		}
		magnitude = magnitude*10 + uint64(digit)
	}

	if negative {
		if magnitude == 1<<63 {
			return -1 << 63, true, nil
		}
		return -int64(magnitude), true, nil
	}
	return int64(magnitude), true, nil
}
