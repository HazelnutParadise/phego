<?php
class A
{
    public $child = null;
    public function hi()
    {
        return "hi";
    }
}
$a = new A;
echo $a->hi(), "\n";
echo $a->child->hi(), "\n";
