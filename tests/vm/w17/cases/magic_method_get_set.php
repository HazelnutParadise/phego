<?php
class Magic
{
    private $data = [];
    public $real = "real";

    public function __get($name)
    {
        echo "__get(", $name, ") ";
        return $this->data[$name] ?? "default for " . $name;
    }

    public function __set($name, $value)
    {
        echo "__set(", $name, ") ";
        $this->data[$name] = $value;
    }
}

$m = new Magic;
echo $m->missing, "\n";
$m->color = "red";
echo "\n";
echo $m->color, "\n";
echo "declared public bypasses: ", $m->real, "\n";
$m->real = "changed";
echo "declared write bypasses: ", $m->real, "\n";
$m->count = 1;
$m->count += 5;
echo "\n";
echo "compound uses get then set: ", $m->count, "\n";
$m->count++;
echo "\n";
echo "increment: ", $m->count, "\n";
$name = "dyn";
$m->$name = "dynamic name";
echo "\n", $m->$name, "\n";
$r = ($m->assigned = "assignment value");
echo "\nassignment expression value: ", $r, "\n";
$m->list = [1];
echo "\n";
$m->list[] = 2;
echo "\nafter indirect modification: ", $m->list[1] ?? "unchanged", "\n";
echo "coalesce calls __isset first (none defined): ", ($m->color ?? "no isset"), "\n";
