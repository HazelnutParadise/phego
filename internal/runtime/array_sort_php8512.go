package runtime

// PHP 8.5.12 comparison schedule, derived from Zend/zend_sort.c.
// REQ-017; T-033/T-034. See third_party/php-sort/UPSTREAM.json and
// THIRD_PARTY_NOTICES.md for pinned source, Zend 2.00 and libc++ notices.
// This derived implementation is not covered solely by the root MIT grant.
func php8512Sort(items []*bucket, compare func(*bucket, *bucket) int) {
	for len(items) > 16 {
		n := len(items)
		middle := n >> 1
		if n>>10 != 0 {
			delta := middle >> 1
			php8512Sort5(items, 0, delta, middle, middle+delta, n-1, compare)
		} else {
			php8512Sort3(items, 0, middle, n-1, compare)
		}

		php8512Swap(items, 1, middle)
		pivot := 1
		i := pivot + 1
		j := n - 1

	partition:
		for {
			for compare(items[pivot], items[i]) > 0 {
				i++
				if i == j {
					break partition
				}
			}
			j--
			if j == i {
				break
			}
			for compare(items[j], items[pivot]) > 0 {
				j--
				if j == i {
					break partition
				}
			}
			php8512Swap(items, i, j)
			i++
			if i == j {
				break
			}
		}

		pivot = i - 1
		php8512Swap(items, 1, pivot)
		right := i
		rightCount := n - right
		if pivot < rightCount {
			php8512Sort(items[:pivot], compare)
			items = items[right:]
		} else {
			php8512Sort(items[right:], compare)
			items = items[:pivot]
		}
	}

	php8512InsertSort(items, compare)
}

func php8512InsertSort(items []*bucket, compare func(*bucket, *bucket) int) {
	switch len(items) {
	case 0, 1:
		return
	case 2:
		php8512Sort2(items, 0, 1, compare)
		return
	case 3:
		php8512Sort3(items, 0, 1, 2, compare)
		return
	case 4:
		php8512Sort4(items, 0, 1, 2, 3, compare)
		return
	case 5:
		php8512Sort5(items, 0, 1, 2, 3, 4, compare)
		return
	}

	n := len(items)
	for i := 1; i < 6; i++ {
		j := i - 1
		if compare(items[j], items[i]) <= 0 {
			continue
		}
		for j != 0 {
			j--
			if compare(items[j], items[i]) <= 0 {
				j++
				break
			}
		}
		for k := i; k > j; k-- {
			php8512Swap(items, k, k-1)
		}
	}

	for i := 6; i < n; i++ {
		j := i - 1
		if compare(items[j], items[i]) <= 0 {
			continue
		}
		for {
			j -= 2
			if compare(items[j], items[i]) <= 0 {
				j++
				if compare(items[j], items[i]) <= 0 {
					j++
				}
				break
			}
			if j == 0 {
				break
			}
			if j == 1 {
				j--
				// Keep Zend's reversed comparison at the left boundary.
				if compare(items[i], items[j]) > 0 {
					j++
				}
				break
			}
		}
		for k := i; k > j; k-- {
			php8512Swap(items, k, k-1)
		}
	}
}

func php8512Sort2(items []*bucket, a, b int, compare func(*bucket, *bucket) int) {
	if compare(items[a], items[b]) > 0 {
		php8512Swap(items, a, b)
	}
}

func php8512Sort3(items []*bucket, a, b, c int, compare func(*bucket, *bucket) int) {
	if compare(items[a], items[b]) <= 0 {
		if compare(items[b], items[c]) <= 0 {
			return
		}
		php8512Swap(items, b, c)
		if compare(items[a], items[b]) > 0 {
			php8512Swap(items, a, b)
		}
		return
	}
	if compare(items[c], items[b]) <= 0 {
		php8512Swap(items, a, c)
		return
	}
	php8512Swap(items, a, b)
	if compare(items[b], items[c]) > 0 {
		php8512Swap(items, b, c)
	}
}

func php8512Sort4(items []*bucket, a, b, c, d int, compare func(*bucket, *bucket) int) {
	php8512Sort3(items, a, b, c, compare)
	if compare(items[c], items[d]) > 0 {
		php8512Swap(items, c, d)
		if compare(items[b], items[c]) > 0 {
			php8512Swap(items, b, c)
			if compare(items[a], items[b]) > 0 {
				php8512Swap(items, a, b)
			}
		}
	}
}

func php8512Sort5(items []*bucket, a, b, c, d, e int, compare func(*bucket, *bucket) int) {
	php8512Sort4(items, a, b, c, d, compare)
	if compare(items[d], items[e]) > 0 {
		php8512Swap(items, d, e)
		if compare(items[c], items[d]) > 0 {
			php8512Swap(items, c, d)
			if compare(items[b], items[c]) > 0 {
				php8512Swap(items, b, c)
				if compare(items[a], items[b]) > 0 {
					php8512Swap(items, a, b)
				}
			}
		}
	}
}

func php8512Swap(items []*bucket, a, b int) {
	items[a], items[b] = items[b], items[a]
}
