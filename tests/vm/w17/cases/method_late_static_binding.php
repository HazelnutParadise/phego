<?php
class Model
{
    public static function create()
    {
        return new static;
    }

    public static function name()
    {
        return static::class;
    }

    public static function selfName()
    {
        return self::class;
    }

    public static function viaSelf()
    {
        return self::name();
    }

    public static function viaStatic()
    {
        return static::name();
    }

    public static function viaClassName()
    {
        return Model::name();
    }

    public function instanceName()
    {
        return static::name() . "/" . $this::class;
    }
}

class User extends Model
{
    public static function viaParent()
    {
        return parent::name();
    }

    public static function viaParentSelf()
    {
        return parent::viaSelf();
    }
}

class Admin extends User
{
}

echo Model::name(), " ", User::name(), " ", Admin::name(), "\n";
echo User::selfName(), "\n";
echo "self:: forwards: ", User::viaSelf(), " ", Admin::viaSelf(), "\n";
echo "static:: ", Admin::viaStatic(), "\n";
echo "ClassName:: does not forward: ", Admin::viaClassName(), "\n";
echo "parent:: forwards: ", Admin::viaParent(), "\n";
echo "parent then self forwards: ", Admin::viaParentSelf(), "\n";
echo "new static: ", Admin::create()::class, " ", User::create()::class, "\n";
echo "instance: ", (new Admin)->instanceName(), "\n";

class Counter
{
    protected static $count = 0;
    public static function inc()
    {
        static::$count++;
        return static::$count;
    }
}
class OwnCounter extends Counter
{
    protected static $count = 100;
}
class SharedCounter extends Counter
{
}
echo Counter::inc(), " ", OwnCounter::inc(), " ", SharedCounter::inc(), " ", Counter::inc(), " ", OwnCounter::inc(), "\n";

class ConstBase
{
    const KIND = "base";
    public function kinds()
    {
        return self::KIND . "/" . static::KIND;
    }
}
class ConstChild extends ConstBase
{
    const KIND = "child";
}
echo (new ConstChild)->kinds(), " ", (new ConstBase)->kinds(), "\n";
