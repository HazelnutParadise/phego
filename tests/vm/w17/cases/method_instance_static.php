<?php
class Calc
{
    public $total = 0;
    public static $calls = 0;

    public function add($n)
    {
        self::$calls++;
        $this->total += $n;
        return $this;
    }

    public static function square($n)
    {
        static::$calls++;
        return $n * $n;
    }

    public function useStatic($n)
    {
        return self::square($n) + static::square($n) + $this->square($n) + Calc::square($n);
    }

    public static function makeAndAdd($n)
    {
        return (new self)->add($n)->total;
    }
}

$c = new Calc;
echo $c->add(2)->add(3)->total, "\n";
echo Calc::square(4), "\n";
echo "instance call of static method: ", $c->square(5), "\n";
echo "static via instance class: ", $c::square(6), "\n";
echo $c->useStatic(2), "\n";
echo Calc::makeAndAdd(9), "\n";
echo "calls: ", Calc::$calls, "\n";
$cls = "Calc";
echo "string class: ", $cls::square(3), "\n";

class Child extends Calc
{
    public function add($n)
    {
        return parent::add($n * 10);
    }

    public static function square($n)
    {
        return 1000 + parent::square($n);
    }
}
$ch = new Child;
echo $ch->add(1)->total, "\n";
echo Child::square(3), "\n";
echo $ch->useStatic(1), "\n";
