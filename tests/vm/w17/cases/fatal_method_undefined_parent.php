<?php
class Base
{
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
