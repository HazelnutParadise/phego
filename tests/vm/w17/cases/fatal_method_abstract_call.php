<?php
abstract class Base
{
    abstract public function m();
}
class Child extends Base
{
    public function m()
    {
        return parent::m();
    }
}
echo "calling\n";
(new Child)->m();
