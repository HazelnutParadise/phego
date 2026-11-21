<?php
class Hybrid
{
    public static $s = "static value";
    public $i = "instance value";

    public function readStaticAsInstance()
    {
        return $this->s;
    }
}

$m = new Hybrid;
echo "instance read of static prop: [", $m->s, "]\n";
$m->s = "dynamic instance prop";
echo "instance write creates instance prop: ", $m->s, " | static: ", Hybrid::$s, "\n";
echo "method: [", (new Hybrid)->readStaticAsInstance(), "]\n";
echo "static via instance class: ", $m::$s, "\n";
