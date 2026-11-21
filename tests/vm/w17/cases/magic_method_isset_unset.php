<?php
class Bag
{
    private $items = ["a" => 1, "nul" => null];

    public function __isset($name)
    {
        echo "__isset(", $name, ") ";
        return isset($this->items[$name]);
    }

    public function __unset($name)
    {
        echo "__unset(", $name, ") ";
        unset($this->items[$name]);
    }

    public function __get($name)
    {
        echo "__get(", $name, ") ";
        return $this->items[$name] ?? null;
    }
}

$b = new Bag;
echo "isset a: ", (isset($b->a) ? "yes" : "no"), "\n";
echo "isset nul: ", (isset($b->nul) ? "yes" : "no"), "\n";
echo "isset zz: ", (isset($b->zz) ? "yes" : "no"), "\n";
echo "empty a: ", (empty($b->a) ? "yes" : "no"), "\n";
echo "empty zz: ", (empty($b->zz) ? "yes" : "no"), "\n";
echo "coalesce a: ", ($b->a ?? "none"), "\n";
echo "coalesce zz: ", ($b->zz ?? "none"), "\n";
echo "nested isset: ", (isset($b->a["k"]) ? "yes" : "no"), "\n";
unset($b->a);
echo "\nafter unset: ", (isset($b->a) ? "yes" : "no"), "\n";

class Truthy
{
    public function __isset($n)
    {
        return 1;
    }
    public function __get($n)
    {
        return "0";
    }
}
$t = new Truthy;
echo "isset truthy int: ", (isset($t->x) ? "yes" : "no"), " empty with falsy value: ", (empty($t->x) ? "yes" : "no"), "\n";
