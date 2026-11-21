<?php
class Left
{
    protected function only()
    {
        return "left";
    }
}
class Right
{
    public function call(Left $l)
    {
        return $l->only();
    }
}
echo "calling\n";
(new Right)->call(new Left);
