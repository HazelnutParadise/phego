<?php
echo "start\n";
class Base
{
    public function run(int $a, string $b = "x"): string
    {
        return "";
    }
}
class Child extends Base
{
    public function run(int $a): string
    {
        return "";
    }
}
