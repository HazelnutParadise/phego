<?php
class Probe
{
    public $set = "v";
    public $null = null;
    public $zero = 0;
    public $emptyStr = "";
    public $arr = ["k" => null, "n" => 1];
    public ?int $typed;
    public int $typedSet = 3;
}

function yn($v)
{
    return $v ? "Y" : "n";
}

$p = new Probe;
echo "isset: set", yn(isset($p->set)), " null", yn(isset($p->null)), " zero", yn(isset($p->zero)),
    " missing", yn(isset($p->missing)), " typedUninit", yn(isset($p->typed)), "\n";
echo "empty: set", yn(empty($p->set)), " null", yn(empty($p->null)), " zero", yn(empty($p->zero)),
    " str", yn(empty($p->emptyStr)), " missing", yn(empty($p->missing)), " typedUninit", yn(empty($p->typed)), "\n";
echo "nested isset: ", yn(isset($p->arr["k"])), yn(isset($p->arr["n"])), yn(isset($p->arr["x"]["y"])), "\n";
echo "multi isset: ", yn(isset($p->set, $p->zero)), yn(isset($p->set, $p->null)), "\n";

unset($p->set);
echo "after unset isset: ", yn(isset($p->set)), "\n";
echo "read after unset: [", $p->set, "]\n";
$p->set = "back";
echo "rewritten: ", $p->set, "\n";
unset($p->arr["n"]);
echo "unset element: ", yn(isset($p->arr["n"])), yn(isset($p->arr["k"])), "\n";
unset($p->missing);
echo "unset missing is silent\n";
unset($p->typedSet);
echo "unset typed isset: ", yn(isset($p->typedSet)), "\n";
$p->typedSet = "7";
echo "typed after re-set: ", $p->typedSet, "\n";

$p->dyn = 1;
unset($p->dyn);
echo "dynamic unset: ", yn(isset($p->dyn)), "\n";
$n = 0;
foreach ($p as $k => $v) {
    $n++;
}
echo "visible initialized props: ", $n, "\n";
echo "read undefined: [", $p->nothing, "]\n";
