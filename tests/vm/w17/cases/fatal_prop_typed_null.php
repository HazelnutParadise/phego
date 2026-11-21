<?php
class User
{
    public string $name = "ann";
}
$u = new User;
echo "name ", $u->name, "\n";
$u->name = null;
