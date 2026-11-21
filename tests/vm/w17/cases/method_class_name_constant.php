<?php
namespace Shop;

class Product
{
    public function names()
    {
        return self::class . " | " . static::class . " | " . $this::class . " | " . __CLASS__;
    }

    public static function staticNames()
    {
        return self::class . " | " . static::class;
    }
}

class Book extends Product
{
    public function parentName()
    {
        return parent::class;
    }
}

$b = new Book;
echo $b->names(), "\n";
echo Book::staticNames(), "\n";
echo $b->parentName(), "\n";
echo Product::class, " ", \Shop\Book::class, " ", \strlen::class, "\n";
echo $b::class, "\n";
$x = $b;
echo "variable: ", $x::class, "\n";
function f()
{
    return __FUNCTION__;
}
echo f(), "\n";
class Methods
{
    public function m()
    {
        return __METHOD__ . " " . __FUNCTION__ . " " . __CLASS__;
    }
    public static function s()
    {
        return __METHOD__;
    }
}
echo (new Methods)->m(), "\n";
echo Methods::s(), "\n";
