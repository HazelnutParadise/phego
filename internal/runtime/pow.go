package runtime

import "github.com/HazelnutParadise/phego/internal/phpnum"

func glibcPow(x, y float64) float64 { return phpnum.Pow(x, y, 64) }
