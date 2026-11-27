<?php
// exception_code_types: getCode for explicit ints, the default, negatives and the integer limits
$a = new Exception("int", 5);
echo "code_5: ", $a->getCode(), "\n";
echo "code_5_is_int: ", ($a->getCode() === 5 ? 1 : 0), "\n";

$b = new Exception("default");
echo "code_default: ", $b->getCode(), "\n";
echo "code_default_is_zero: ", ($b->getCode() === 0 ? 1 : 0), "\n";

$c = new Exception("neg", -5);
echo "code_negative: ", $c->getCode(), "\n";

$d = new Exception("max", PHP_INT_MAX);
echo "code_max: ", $d->getCode(), "\n";
echo "code_max_is_int: ", ($d->getCode() === PHP_INT_MAX ? 1 : 0), "\n";

$f = new Exception("min", PHP_INT_MIN);
echo "code_min: ", $f->getCode(), "\n";

$g = new Error("err code", 404);
echo "error_code: ", $g->getCode(), "\n";

$i = new Exception("zero", 0);
echo "explicit_zero: ", $i->getCode(), "\n";
echo "sum_check: ", $a->getCode() + $c->getCode(), "\n";
echo "negative_cmp: ", ($c->getCode() < $i->getCode() ? 1 : 0), "\n";

$h = new Exception("big", 1000000);
echo "code_million: ", $h->getCode(), "\n";
echo "code_million_plus_one: ", $h->getCode() + 1, "\n";
echo "end\n";
