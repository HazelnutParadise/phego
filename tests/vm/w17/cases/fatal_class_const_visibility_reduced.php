<?php
echo "start\n";
class Base
{
    public const NAME = "a";
}
class Child extends Base
{
    protected const NAME = "b";
}
