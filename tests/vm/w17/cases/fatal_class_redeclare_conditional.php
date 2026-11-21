<?php
class Thing
{
    const V = 1;
}
echo "first declared ", Thing::V, "\n";
if (Thing::V === 1) {
    class Thing
    {
    }
}
echo "not reached\n";
