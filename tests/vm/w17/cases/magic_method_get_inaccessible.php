<?php
class Guarded
{
    private $secret = "private value";
    protected $prot = "protected value";
    public $open = "public value";

    public function __get($name)
    {
        return "__get " . $name;
    }

    public function __set($name, $v)
    {
        echo "__set ", $name, "\n";
    }

    public function inside()
    {
        return $this->secret . " | " . $this->prot;
    }
}

class Sub extends Guarded
{
    public function fromChild()
    {
        return $this->secret . " | " . $this->prot;
    }
}

$g = new Guarded;
echo $g->secret, "\n";
echo $g->prot, "\n";
echo $g->open, "\n";
echo $g->inside(), "\n";
$g->secret = "attempt";
echo $g->inside(), "\n";
echo (new Sub)->fromChild(), "\n";

class Typed
{
    public int $lazy;
    public function __get($n)
    {
        echo "__get for unset typed ", $n, ": ";
        return "42";
    }
}
$t = new Typed;
unset($t->lazy);
echo $t->lazy, "\n";
