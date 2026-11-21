<?php
class Name
{
    public function __toString()
    {
        return "name";
    }
}
echo "parent works: ", new Name, "\n";
echo new LaterChild, "\n";
class LaterChild extends Name
{
}
