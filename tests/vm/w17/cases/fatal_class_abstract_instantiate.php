<?php
abstract class Shape
{
    public static function kind()
    {
        return "shape";
    }
}
echo Shape::kind(), "\n";
$s = new Shape();
echo "not reached\n";
