<?php
// Calling new with first-class callable syntax is not allowed.
// The whole file fails to compile, so none of the code below runs.

class A
{
    public function __construct($v = 0)
    {
        $this->v = $v;
    }

    public function get()
    {
        return $this->v;
    }
}

$a = new A(5);
echo "constructed: ", $a->get(), "\n";

$plain = new A();
echo "default constructed: ", $plain->get(), "\n";

// Invalid: (...) cannot follow new.
$bad = new A(...);

echo "not reached\n";
