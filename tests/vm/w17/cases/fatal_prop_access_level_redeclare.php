<?php
echo "start\n";
class Base
{
    public $v = 1;
}
class Child extends Base
{
    protected $v = 2;
}
