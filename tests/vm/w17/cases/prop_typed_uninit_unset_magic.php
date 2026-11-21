<?php
class Lazy
{
    public int $count;
    public string $name = "x";

    public function __construct()
    {
        unset($this->name);
    }

    public function __get($n)
    {
        echo "__get(", $n, ") ";
        $this->$n = $n === "name" ? "lazy name" : 7;
        return $this->$n;
    }
}

$l = new Lazy;
echo "never-initialized typed: isset ", (isset($l->count) ? "yes" : "no"), "\n";
echo "unset typed triggers __get: ", $l->name, "\n";
echo "second read no __get: ", $l->name, "\n";
$l->count = 3;
echo "count: ", $l->count, "\n";
