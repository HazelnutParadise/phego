package runtime

import "github.com/HazelnutParadise/phego/internal/phpnum"

func i386Pow(x, y float64) float64 { return phpnum.Pow(x, y, 32) }
