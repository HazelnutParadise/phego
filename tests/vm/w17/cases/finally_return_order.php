<?php
// finally_return_order: the return value is computed before finally; finally changing the returned variable does not change it, but mutating a returned object is visible
class Box
{
    public $v;
    function __construct($v) { $this->v = $v; }
}

function note($s)
{
    echo "note: ", $s, "\n";
    return $s;
}

function scalar()
{
    $x = 1;
    try {
        echo "scalar try\n";
        return $x;
    } finally {
        $x = 99;
        echo "scalar finally sees x=", $x, "\n";
    }
}
$r = scalar();
echo "scalar result: ", $r, "\n";

function arrayCase()
{
    $a = [1, 2];
    try {
        return $a;
    } finally {
        $a[] = 3;
        echo "array finally changed local\n";
    }
}
$r = arrayCase();
echo "array result: ", $r[0], $r[1], "\n";
echo "array result has third: ", isset($r[2]) ? "yes" : "no", "\n";

function objectCase()
{
    $b = new Box("old");
    try {
        return $b;
    } finally {
        $b->v = "new";
        echo "object finally mutated\n";
    }
}
$o = objectCase();
echo "object result v: ", $o->v, "\n";

function exprOrder()
{
    try {
        return note("expr");
    } finally {
        echo "exprOrder finally\n";
    }
}
$r = exprOrder();
echo "exprOrder result: ", $r, "\n";

function incrementReturn()
{
    $n = 0;
    try {
        return ++$n;
    } finally {
        echo "increment finally n=", $n, "\n";
    }
}
$r = incrementReturn();
echo "increment result: ", $r, "\n";
