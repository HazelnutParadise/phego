<?php
class Base
{
    protected function inner()
    {
        return "inner";
    }
}
function outside(Base $b)
{
    return $b->inner();
}
echo "calling\n";
outside(new Base);
