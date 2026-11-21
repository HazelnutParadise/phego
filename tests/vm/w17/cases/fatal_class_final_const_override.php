<?php
echo "start\n";
class Base
{
    final public const LIMIT = 1;
}
class Child extends Base
{
    public const LIMIT = 2;
}
