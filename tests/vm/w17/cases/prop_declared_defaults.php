<?php
const APP = "app";

class Defaults
{
    const BASE = 10;
    public $none;
    public $null = null;
    public $int = 1;
    public $neg = -5;
    public $float = 1.5;
    public $str = "text";
    public $bool = false;
    public $arr = [1, "k" => "v", [2, 3]];
    public $expr = 2 * 3 + 1;
    public $concat = "a" . "b" . APP;
    public $fromConst = self::BASE * 2;
    public $fromOther = Other::VALUE;
    public $magic = __CLASS__;
    public $line = __LINE__;
    public $nestedConst = [self::BASE => Other::VALUE];
    public $ternary = self::BASE > 5 ? "big" : "small";
    public $a = 1, $b = 2, $c;
}

class Other
{
    const VALUE = "other value";
}

$d = new Defaults;
echo "none is null: ", ($d->none === null ? "yes" : "no"), "\n";
echo "null: ", ($d->null === null ? "yes" : "no"), "\n";
echo $d->int, " ", $d->neg, " ", $d->float, " ", $d->str, " [", $d->bool, "]\n";
echo $d->arr[0], " ", $d->arr["k"], " ", $d->arr[1][1], "\n";
echo $d->expr, " ", $d->concat, " ", $d->fromConst, "\n";
echo $d->fromOther, "\n";
echo $d->magic, " line ", $d->line, "\n";
echo $d->nestedConst[10], " ", $d->ternary, "\n";
echo $d->a, $d->b, ($d->c === null ? "null" : "set"), "\n";

$d->arr[] = "appended";
$e = new Defaults;
echo "defaults are per instance: ", (isset($e->arr[2]) ? "has" : "no"), " ", (isset($d->arr[2]) ? "has" : "no"), "\n";

class Child extends Defaults
{
    public $int = 100;
    public $str;
}
$c = new Child;
echo "child override: ", $c->int, " str null: ", ($c->str === null ? "yes" : "no"), " inherited: ", $c->float, " magic: ", $c->magic, "\n";
