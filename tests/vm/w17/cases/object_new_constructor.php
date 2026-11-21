<?php
class Box
{
    public $items = [];
    public $label;

    public function __construct($label = "box", ...$items)
    {
        echo "ctor ", $label, "\n";
        $this->label = $label;
        $this->items = $items;
    }

    public function show()
    {
        $out = $this->label . ":";
        foreach ($this->items as $i) {
            $out .= " " . $i;
        }
        return $out;
    }
}

echo (new Box)->show(), "\n";
echo (new Box())->show(), "\n";
echo (new Box("tools", "hammer", "saw"))->show(), "\n";
$args = ["fruit", "apple", "pear"];
echo (new Box(...$args))->show(), "\n";
echo (new Box(label: "named"))->show(), "\n";
$keyed = ["label" => "keyed"];
echo (new Box(...$keyed))->show(), "\n";

class Pair
{
    public function __construct(public $left = "L", public $right = "R")
    {
    }
}
$p = new Pair(right: "right only");
echo $p->left, " ", $p->right, "\n";
$p = new Pair(...["right" => "r2", "left" => "l2"]);
echo $p->left, " ", $p->right, "\n";

function side_effect($v)
{
    echo "arg ", $v, "\n";
    return $v;
}
$b = new Box(side_effect("first"), side_effect("second"));
echo $b->show(), "\n";

class NoCtor
{
    public $v = "no ctor";
}
$n = new NoCtor(side_effect("ignored arg is still evaluated"));
echo $n->v, "\n";

class Explicit
{
    public $calls = 0;
    public function __construct()
    {
        $this->calls++;
    }
}
$e = new Explicit;
$e->__construct();
echo "explicit ctor calls: ", $e->calls, "\n";

$value = new Box("in expression") instanceof Box;
echo "new in expression: ", ($value ? "yes" : "no"), "\n";
