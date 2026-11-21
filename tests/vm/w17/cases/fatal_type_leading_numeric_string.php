<?php
function i(int $x)
{
    return $x;
}
echo "numeric string ok: ", i("12"), "\n";
echo i("12abc"), "\n";
echo "not reached\n";
