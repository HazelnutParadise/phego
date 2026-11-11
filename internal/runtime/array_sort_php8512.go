package runtime

// PHP 8.5.12 comparison schedule, derived from Zend/zend_sort.c.
// REQ-017; T-033/T-034. See third_party/php-sort/UPSTREAM.json and
// THIRD_PARTY_NOTICES.md for pinned source, Zend 2.00 and libc++ notices.
// This derived implementation is not covered solely by the root MIT grant.
// It sorts positions 0..n-1 through compare and swap, as zend_sort does
// through its cmp and swp callbacks; the caller decides what a swap moves.
func php8512Sort(n int, compare func(i, j int) int, swap func(i, j int)) {
	php8512Sorter{compare, swap}.sort(0, n)
}

type php8512Sorter struct {
	compare func(i, j int) int
	swap    func(i, j int)
}

// sort orders the n positions starting at base.
func (s php8512Sorter) sort(base, n int) {
	for n > 16 {
		middle := n >> 1
		if n>>10 != 0 {
			delta := middle >> 1
			s.sort5(base, 0, delta, middle, middle+delta, n-1)
		} else {
			s.sort3(base, 0, middle, n-1)
		}

		s.swap(base+1, base+middle)
		pivot := 1
		i := pivot + 1
		j := n - 1

	partition:
		for {
			for s.compare(base+pivot, base+i) > 0 {
				i++
				if i == j {
					break partition
				}
			}
			j--
			if j == i {
				break
			}
			for s.compare(base+j, base+pivot) > 0 {
				j--
				if j == i {
					break partition
				}
			}
			s.swap(base+i, base+j)
			i++
			if i == j {
				break
			}
		}

		pivot = i - 1
		s.swap(base+1, base+pivot)
		right := i
		rightCount := n - right
		if pivot < rightCount {
			s.sort(base, pivot)
			base, n = base+right, rightCount
		} else {
			s.sort(base+right, rightCount)
			n = pivot
		}
	}

	s.insertSort(base, n)
}

func (s php8512Sorter) insertSort(base, n int) {
	switch n {
	case 0, 1:
		return
	case 2:
		s.sort2(base, 0, 1)
		return
	case 3:
		s.sort3(base, 0, 1, 2)
		return
	case 4:
		s.sort4(base, 0, 1, 2, 3)
		return
	case 5:
		s.sort5(base, 0, 1, 2, 3, 4)
		return
	}

	for i := 1; i < 6; i++ {
		j := i - 1
		if s.compare(base+j, base+i) <= 0 {
			continue
		}
		for j != 0 {
			j--
			if s.compare(base+j, base+i) <= 0 {
				j++
				break
			}
		}
		for k := i; k > j; k-- {
			s.swap(base+k, base+k-1)
		}
	}

	for i := 6; i < n; i++ {
		j := i - 1
		if s.compare(base+j, base+i) <= 0 {
			continue
		}
		for {
			j -= 2
			if s.compare(base+j, base+i) <= 0 {
				j++
				if s.compare(base+j, base+i) <= 0 {
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
				if s.compare(base+i, base+j) > 0 {
					j++
				}
				break
			}
		}
		for k := i; k > j; k-- {
			s.swap(base+k, base+k-1)
		}
	}
}

func (s php8512Sorter) sort2(base, a, b int) {
	if s.compare(base+a, base+b) > 0 {
		s.swap(base+a, base+b)
	}
}

func (s php8512Sorter) sort3(base, a, b, c int) {
	if s.compare(base+a, base+b) <= 0 {
		if s.compare(base+b, base+c) <= 0 {
			return
		}
		s.swap(base+b, base+c)
		if s.compare(base+a, base+b) > 0 {
			s.swap(base+a, base+b)
		}
		return
	}
	if s.compare(base+c, base+b) <= 0 {
		s.swap(base+a, base+c)
		return
	}
	s.swap(base+a, base+b)
	if s.compare(base+b, base+c) > 0 {
		s.swap(base+b, base+c)
	}
}

func (s php8512Sorter) sort4(base, a, b, c, d int) {
	s.sort3(base, a, b, c)
	if s.compare(base+c, base+d) > 0 {
		s.swap(base+c, base+d)
		if s.compare(base+b, base+c) > 0 {
			s.swap(base+b, base+c)
			if s.compare(base+a, base+b) > 0 {
				s.swap(base+a, base+b)
			}
		}
	}
}

func (s php8512Sorter) sort5(base, a, b, c, d, e int) {
	s.sort4(base, a, b, c, d)
	if s.compare(base+d, base+e) > 0 {
		s.swap(base+d, base+e)
		if s.compare(base+c, base+d) > 0 {
			s.swap(base+c, base+d)
			if s.compare(base+b, base+c) > 0 {
				s.swap(base+b, base+c)
				if s.compare(base+a, base+b) > 0 {
					s.swap(base+a, base+b)
				}
			}
		}
	}
}
