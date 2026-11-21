<?php
// A static closure made in a method that uses $this.
// The result of PHP 8.5.12 decides which kind of error is reported.

class Holder
{
    public $label = "held";

    public function make()
    {
        echo "making a static closure\n";
        return static function () {
            return $this->label;
        };
    }
}

$h = new Holder();
echo "holder label: ", $h->label, "\n";

$f = $h->make();

echo "calling the static closure\n";
echo "result: ", $f(), "\n";

echo "not reached\n";
