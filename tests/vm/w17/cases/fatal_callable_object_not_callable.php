<?php
// An object without __invoke is not callable. Creating a callable from it is
// a fatal error at creation time.

class HasInvoke
{
    public function __invoke($v)
    {
        return "invoked with " . $v;
    }
}

$good = new HasInvoke();
$c = $good(...);
echo "object with __invoke: ", $c("a"), "\n";

echo "direct invoke: ", $good("b"), "\n";

$o = new stdClass();
echo "stdClass object created\n";
echo "before calling the stdClass object\n";

$bad = $o(...);

echo "after creation, not reached\n";
