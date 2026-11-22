<?php
// A top-level arrow function that uses $this. There is no object at top level,
// so calling the arrow function is a fatal error.

class Pod
{
    public $x = 42;
}

$pod = new Pod();
echo "pod x: ", $pod->x, "\n";

$plain = fn($n) => $n * 10;
echo "plain arrow: ", $plain(3), "\n";

$reader = fn() => $this->x;

echo "about to call the arrow function that uses this\n";

echo "value: ", $reader(), "\n";

echo "not reached\n";
