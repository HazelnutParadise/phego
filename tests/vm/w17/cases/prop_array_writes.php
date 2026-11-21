<?php
class Store
{
    public $list = [];
    public $map;
    public $str = "abc";
    public ?array $typed = null;
}

$s = new Store;
$s->list[] = 1;
$s->list[] = 2;
$s->list["k"]["j"] = "deep";
$s->list["k"]["j2"][] = "deeper";
echo $s->list[0], $s->list[1], " ", $s->list["k"]["j"], " ", $s->list["k"]["j2"][0], "\n";

$s->map["auto"] = "vivified from null";
echo $s->map["auto"], "\n";
$s->map["n"] = 1;
$s->map["n"] += 5;
$s->map["n"]++;
$s->map["c"] ??= "coalesced";
$s->map["c"] ??= "not used";
$s->map["s"] = "x";
$s->map["s"] .= "y";
echo $s->map["n"], " ", $s->map["c"], " ", $s->map["s"], "\n";

$s->typed[] = "typed nullable array vivified";
echo $s->typed[0], "\n";

$s->str[0] = "X";
echo "string offset write: ", $s->str, "\n";
echo "string offset read: ", $s->str[2], "\n";

$s->list = [[1, 2], [3, 4]];
foreach ($s->list as $i => $row) {
    $s->list[$i][] = $i * 10;
}
echo $s->list[0][2], " ", $s->list[1][2], "\n";
[$s->list[0], $s->list[1]] = [$s->list[1], $s->list[0]];
echo "swap via list(): ", $s->list[0][0], $s->list[1][0], "\n";
[$s->str, $s->map] = ["destructured", null];
echo $s->str, " ", ($s->map === null ? "null" : "set"), "\n";

$t = new Store;
$copy = $t->list;
$copy[] = "only in copy";
echo "property array is a value: ", (isset($t->list[0]) ? "shared" : "separate"), "\n";
$t->list[5] = "five";
$t->list[] = "six";
foreach ($t->list as $k => $v) {
    echo $k, "=", $v, " ";
}
echo "\n";
