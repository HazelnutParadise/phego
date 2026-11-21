<?php
echo "start\n";
trait A
{
    public function hello()
    {
        return "A";
    }
}
trait B
{
    public function hello()
    {
        return "B";
    }
}
class Talker
{
    use A, B;
}
