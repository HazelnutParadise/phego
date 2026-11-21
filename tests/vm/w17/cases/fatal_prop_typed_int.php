<?php
class User
{
    public int $age = 0;
}
$u = new User;
$u->age = "30";
echo "numeric string accepted: ", $u->age, "\n";
$u->age = "thirty";
echo "not reached\n";
