<?php
echo "before\n";
class NotATrait
{
}
class Wrong
{
    use NotATrait;
}
echo "not reached\n";
