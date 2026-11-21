<?php
class Base
{
    final public function id()
    {
        return "Base::id for " . $this->name();
    }

    public function name()
    {
        return "base";
    }

    final public static function make()
    {
        return new static();
    }
}

class Child extends Base
{
    public function name()
    {
        return "child";
    }
}

final class Leaf extends Child
{
    final public function extra()
    {
        return "final method in final class";
    }
}

$b = new Base;
$c = new Child;
$l = new Leaf;
echo $b->id(), "\n";
echo $c->id(), "\n";
echo $l->id(), "\n";
echo $l->extra(), "\n";
echo Child::make()->name(), "\n";
echo Leaf::make()->name(), "\n";

class WithFinalConst
{
    final public const LIMIT = 10;
    public const OPEN = 1;
}
class OverridesOpen extends WithFinalConst
{
    public const OPEN = 2;
}
echo WithFinalConst::LIMIT, " ", OverridesOpen::LIMIT, " ", WithFinalConst::OPEN, " ", OverridesOpen::OPEN, "\n";

class PrivateFinalAllowed
{
    private function helper()
    {
        return "parent private";
    }
    public function call()
    {
        return $this->helper();
    }
}
class PrivateFinalChild extends PrivateFinalAllowed
{
    private function helper()
    {
        return "child private";
    }
    public function callChild()
    {
        return $this->helper();
    }
}
$p = new PrivateFinalChild;
echo $p->call(), " / ", $p->callChild(), "\n";
