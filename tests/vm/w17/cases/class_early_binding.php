<?php
echo "before declaration: ", (new Hoisted)->hi(), "\n";
echo "child of earlier parent used early: ", (new EarlyChild)->hi(), "\n";

class Hoisted
{
    public function hi()
    {
        return "hoisted";
    }
}

class EarlyChild extends Hoisted
{
    public function hi()
    {
        return "early child of " . parent::hi();
    }
}

class LateChild extends LateParent
{
    public function hi()
    {
        return "late child of " . parent::hi();
    }
}

class LateParent
{
    public function hi()
    {
        return "late parent";
    }
}

echo "after both declarations: ", (new LateChild)->hi(), "\n";

function uses_class_declared_later()
{
    return (new DeclaredAfterFunction)->v;
}
echo uses_class_declared_later(), "\n";

class DeclaredAfterFunction
{
    public $v = "class declared after the function";
}

class Impl implements EarlyIface
{
    public function m()
    {
        return "impl";
    }
}
interface EarlyIface
{
    public function m();
}
echo "class implementing a later interface, used after both: ", (new Impl)->m(), "\n";
