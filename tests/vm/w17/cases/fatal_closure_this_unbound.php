<?php
// Runtime rule: a closure that uses $this and is not bound to an object
// fails when it runs, because there is no object to use.
// The closure is created at top level, so it has no $this.

class Box
{
    public $x = 7;
}

$box = new Box();
echo "box x: ", $box->x, "\n";

$plain = function () {
    return "no this needed";
};
echo "plain closure: ", $plain(), "\n";

$reader = function () {
    return $this->x;
};

echo "about to call the unbound closure\n";

$value = $reader();

echo "not reached: ", $value, "\n";
