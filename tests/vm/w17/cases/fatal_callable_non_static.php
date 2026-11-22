<?php
// A non-static method is referenced as a static callable from outside any
// object. The error comes when the callable is created.

class A
{
    public function inst()
    {
        return "instance method";
    }

    public static function stat()
    {
        return "static method";
    }
}

echo "static call: ", A::stat(), "\n";

$s = A::stat(...);
echo "static callable: ", $s(), "\n";

$a = new A();
echo "instance call: ", $a->inst(), "\n";

echo "before the bad static reference\n";

$bad = A::inst(...);

echo "after creation, not reached\n";
