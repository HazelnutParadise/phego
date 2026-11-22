<?php
// $obj->nope(...) on a class without __call. The error comes when the callable
// is created, not when it is called.

class Plain
{
    public function real()
    {
        return "real method";
    }
}

$obj = new Plain();
$r = $obj->real(...);
echo "existing method: ", $r(), "\n";

$other = new Plain();
echo "other object: ", $other->real(), "\n";

echo "before the bad first-class callable\n";

$bad = $obj->nope(...);

echo "after creation, not reached\n";

echo $bad(), "\n";
