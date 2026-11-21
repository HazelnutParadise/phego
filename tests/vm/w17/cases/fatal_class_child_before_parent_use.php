<?php
echo "start\n";
echo (new LateChild)->hi(), "\n";
class LateChild extends LateParent
{
    public function hi()
    {
        return "child";
    }
}
class LateParent
{
}
