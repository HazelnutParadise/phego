<?php
echo "class re-implementing an interface may override its constant\n";
interface Limits
{
    const MAX = 10;
}
class Base implements Limits
{
}
class Child extends Base implements Limits
{
    const MAX = 20;
}
echo Child::MAX, "\n";
