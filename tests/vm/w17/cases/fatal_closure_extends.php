<?php
// Runtime rule: Closure is a final class, so it cannot be extended.
// The class is declared inside a block, so PHP reports the fatal error when
// the declaration runs, after the line below has printed.

class Base
{
    public function hello()
    {
        return "hello from Base";
    }
}

$b = new Base();
echo $b->hello(), "\n";

$f = function () {
    return "closure ok";
};
echo $f(), "\n";

echo "about to declare a subclass of Closure\n";

if (true) {
    class ClosureChild extends Closure
    {
    }
}

echo "not reached\n";
