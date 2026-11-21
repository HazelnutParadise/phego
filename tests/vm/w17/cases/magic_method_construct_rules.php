<?php
class Base
{
    public $log = "";
    public function __construct($v = "base default")
    {
        $this->log .= "Base(" . $v . ")";
    }
}

class InheritsCtor extends Base
{
}

class OverridesCtor extends Base
{
    public function __construct()
    {
        $this->log .= "Overrides";
        parent::__construct("from child");
    }
}

class SkipsParent extends Base
{
    public function __construct()
    {
        $this->log .= "Skips";
    }
}

class CtorReturns
{
    public $v = "object";
    public function __construct()
    {
        return "ignored";
    }
}

echo (new InheritsCtor("given"))->log, "\n";
echo (new InheritsCtor)->log, "\n";
echo (new OverridesCtor)->log, "\n";
echo (new SkipsParent)->log, "\n";
echo (new CtorReturns)->v, "\n";

class Legacy
{
    public $v = "not called";
    public function Legacy()
    {
        $this->v = "old style constructor called";
    }
}
echo "old-style constructor is a plain method: ", (new Legacy)->v, "\n";

class PrivateCtor
{
    private function __construct(public $how)
    {
    }

    public static function make()
    {
        return new self("factory");
    }
}
echo PrivateCtor::make()->how, "\n";
