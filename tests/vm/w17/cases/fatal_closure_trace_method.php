<?php
// An error raised inside a closure made in a method. The closure is called from
// another method, and also from a static closure of the same class. The stack
// trace shows the closure frames with the arguments that were passed.

class Worker
{
    private $tag = "w";

    public function make()
    {
        return function ($n) {
            return $this->tag . $n . missing_action($n);
        };
    }

    public function run($n)
    {
        $f = $this->make();
        return $f($n);
    }

    public static function build()
    {
        return static function ($m) {
            $w = new Worker();
            return $w->run($m);
        };
    }
}

$worker = new Worker();
echo "method made closure ready\n";

$s = Worker::build();
echo "static closure ready\n";
echo "calling from a static closure: ", $s(5), "\n";

echo "not reached\n";
