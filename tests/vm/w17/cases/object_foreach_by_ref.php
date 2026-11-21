<?php
class Bag
{
    public $x = 1;
    public $y = 2;
    private $z = 3;
    public $list = [1, 2];

    public function doubleAll()
    {
        foreach ($this as $k => &$v) {
            if ($k !== "list") {
                $v *= 2;
            }
        }
        unset($v);
    }

    public function z()
    {
        return $this->z;
    }
}

$b = new Bag;
foreach ($b as $k => &$v) {
    if ($k !== "list") {
        $v = $v * 10;
    }
}
unset($v);
echo $b->x, " ", $b->y, " ", $b->z(), "\n";
$b->doubleAll();
echo $b->x, " ", $b->y, " ", $b->z(), "\n";

foreach ($b->list as &$item) {
    $item += 100;
}
unset($item);
echo $b->list[0], " ", $b->list[1], "\n";

foreach ($b as $k => &$v) {
    if ($k === "x") {
        $v = "replaced";
    }
}
echo "ref still bound after loop: ";
$v = "via leftover ref";
echo $b->list === "via leftover ref" ? "list" : "other", "\n";
unset($v);

$b2 = new Bag;
foreach ($b2 as $k => $v) {
    $b2->$k = "overwritten";
}
echo "by-value writes through name: ", $b2->x, " ", $b2->list, "\n";

$b3 = new Bag;
foreach ($b3 as $k => &$v) {
    if ($k === "x") {
        $b3->added = "new";
    }
    echo $k, " ";
}
unset($v);
echo "\n";
