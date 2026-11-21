<?php
class Base
{
}
class Left extends Base
{
    protected const SECRET = "left";
}
class Right extends Base
{
    public function peek()
    {
        return Left::SECRET;
    }
}
echo "peeking\n";
echo (new Right)->peek(), "\n";
