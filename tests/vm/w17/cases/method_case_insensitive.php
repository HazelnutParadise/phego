<?php
class Greeter
{
    public function sayHello($n)
    {
        return "hello " . $n;
    }

    public static function MakeOne()
    {
        return new GREETER;
    }

    public function callLower()
    {
        return $this->sayhello("lower") . " / " . self::makeone()->SAYHELLO("upper");
    }
}

$g = new Greeter;
echo $g->sayHello("a"), "\n";
echo $g->SAYHELLO("b"), "\n";
echo $g->sayhello("c"), "\n";
echo greeter::makeONE()->SayHello("d"), "\n";
echo $g->callLower(), "\n";
$m = "SAYhello";
echo $g->$m("dynamic"), "\n";

class Sub extends Greeter
{
    public function SAYHELLO($n)
    {
        return "sub " . parent::sayhello($n);
    }
}
echo (new Sub)->sayHello("override by other case"), "\n";
