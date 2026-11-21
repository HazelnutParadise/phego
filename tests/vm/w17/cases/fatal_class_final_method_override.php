<?php
echo "start\n";
class Base
{
    final public function locked()
    {
    }
}
class Child extends Base
{
    public function locked()
    {
    }
}
