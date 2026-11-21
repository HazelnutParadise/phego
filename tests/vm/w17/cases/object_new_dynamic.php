<?php
class Base
{
    public $tag = "base";

    public static function create()
    {
        return new static();
    }

    public static function createSelf()
    {
        return new self();
    }

    public function cloneKind()
    {
        return new static;
    }
}

class Derived extends Base
{
    public $tag = "derived";

    public function parentInstance()
    {
        return new parent;
    }
}

echo Base::create()->tag, " ", Derived::create()->tag, "\n";
echo Derived::createSelf()->tag, "\n";
$d = new Derived;
echo $d->cloneKind()->tag, " ", $d->parentInstance()->tag, "\n";

$name = "Derived";
$o = new $name;
echo "string: ", $o->tag, "\n";
$o = new $name();
echo "string with parens: ", $o::class, "\n";
$holder = ["cls" => "Base"];
$o = new $holder["cls"];
echo "array element: ", $o::class, "\n";
$other = new $o;
echo "from object: ", $other::class, " same: ", ($other === $o ? "yes" : "no"), "\n";

class Holder
{
    public $cls = "Derived";
    public static $staticCls = "Base";
}
$h = new Holder;
$o = new $h->cls;
echo "property: ", $o::class, "\n";
$o = new Holder::$staticCls;
echo "static property: ", $o::class, "\n";

echo "class constant on object: ", $d::class, "\n";
echo "static::class ", Derived::create()::class, "\n";
$lower = "derived";
echo "case-insensitive string: ", (new $lower)::class, "\n";
$ns = "\\Base";
echo "leading backslash: ", (new $ns)::class, "\n";
