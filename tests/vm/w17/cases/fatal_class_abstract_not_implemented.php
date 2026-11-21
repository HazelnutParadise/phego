<?php
echo "start\n";
abstract class Shape
{
    abstract public function area();
    abstract public function perimeter();
}
class Circle extends Shape
{
    public function area()
    {
        return 3;
    }
}
