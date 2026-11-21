<?php
echo "start\n";
class Root
{
    public function m()
    {
        return parent::m();
    }
}
