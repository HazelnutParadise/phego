<?php
function count_up($n)
{
    return $n->next();
}
echo "calling\n";
count_up(3);
