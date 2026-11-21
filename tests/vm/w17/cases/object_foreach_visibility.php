<?php
class Base
{
    public $a = "A";
    protected $b = "B";
    private $c = "C";
    public $d;

    public function inside()
    {
        $out = "";
        foreach ($this as $k => $v) {
            $out .= $k . "=" . ($v ?? "null") . " ";
        }
        return $out;
    }

    public function other(Base $o)
    {
        $out = "";
        foreach ($o as $k => $v) {
            $out .= $k . " ";
        }
        return $out;
    }
}

class Child extends Base
{
    public $e = "E";
    private $f = "F";

    public function childInside()
    {
        $out = "";
        foreach ($this as $k => $v) {
            $out .= $k . " ";
        }
        return $out;
    }
}

$b = new Base;
echo "outside: ";
foreach ($b as $k => $v) {
    echo $k, "=", ($v ?? "null"), " ";
}
echo "\n";
echo "inside: ", $b->inside(), "\n";
$ch = new Child;
$ch->dyn = "dynamic";
echo "child outside: ";
foreach ($ch as $k => $v) {
    echo $k, " ";
}
echo "\n";
echo "child via base method: ", $ch->inside(), "\n";
echo "child via child method: ", $ch->childInside(), "\n";
echo "base method on other child: ", $b->other($ch), "\n";

function local_iter($o)
{
    $n = 0;
    foreach ($o as $v) {
        $n++;
    }
    return $n;
}
echo "count outside: ", local_iter($b), " ", local_iter($ch), "\n";

$b->a = "changed";
foreach ($b as $k => $v) {
    if ($k === "a") {
        $b->d = "set during loop";
    }
    echo $k, ":", ($v ?? "null"), " ";
}
echo "\n";
echo "after loop d=", $b->d, "\n";
