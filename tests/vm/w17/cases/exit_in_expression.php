<?php
function stop()
{
    exit(9);
}
echo "a", "b", stop(), "c";
echo "never";
