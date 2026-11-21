<?php
class Counter
{
    public $n = 0;
    public static function bump()
    {
        echo "in static\n";
        return $this->n++;
    }
}
Counter::bump();
