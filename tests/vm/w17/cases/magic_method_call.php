<?php
class Proxy
{
    public function __call($name, $args)
    {
        $out = "__call " . $name . "(";
        foreach ($args as $k => $v) {
            $out .= $k . "=" . $v . " ";
        }
        return $out . ")";
    }

    public static function __callStatic($name, $args)
    {
        $out = "__callStatic " . $name . "(";
        foreach ($args as $v) {
            $out .= $v . " ";
        }
        return $out . ")";
    }

    private function hidden($x)
    {
        return "hidden " . $x;
    }

    protected static function shielded()
    {
        return "shielded";
    }

    public function realMethod()
    {
        return "real " . $this->hidden("from inside");
    }

    public function callStaticInside()
    {
        return self::unknownStatic(1);
    }
}

$p = new Proxy;
echo $p->anything(1, "two"), "\n";
echo $p->noArgs(), "\n";
echo $p->named(a: 1, b: 2), "\n";
echo $p->spread(...[3, 4]), "\n";
echo Proxy::staticThing("x", "y"), "\n";
echo "private from outside: ", $p->hidden("outside"), "\n";
echo "protected static from outside: ", Proxy::shielded(), "\n";
echo $p->realMethod(), "\n";
$m = "dynamicName";
echo $p->$m(5), "\n";
echo "self:: inside instance with __call: ", $p->callStaticInside(), "\n";
echo "case preserved: ", $p->MiXeD(), "\n";

class Child extends Proxy
{
    public function viaParent()
    {
        return parent::missingOnParent("p");
    }
}
echo (new Child)->viaParent(), "\n";
echo Child::fromChild(1), "\n";
