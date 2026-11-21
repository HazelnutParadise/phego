<?php
class Math
{
    public static function sq(float $x): float
    {
        return $x * $x;
    }
}
echo Math::sq("3"), "\n";
echo Math::sq("three"), "\n";
