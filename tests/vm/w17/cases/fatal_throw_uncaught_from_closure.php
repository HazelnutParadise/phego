<?php
// fatal_throw_uncaught_from_closure: uncaught exception thrown in a closure that is called through an arrow function and a method
class Runner
{
    public function go(callable $c)
    {
        return $c();
    }
}

$fail = function () {
    throw new Exception("from closure");
};
$arrow = fn() => $fail();
$runner = new Runner();

echo "start\n";
$runner->go($arrow);
echo "never\n";
