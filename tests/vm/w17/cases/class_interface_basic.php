<?php
interface HasName
{
    const PREFIX = "name:";
    public function name();
}

interface HasSize
{
    const UNIT = "kb";
    public function size();
}

interface Describable extends HasName, HasSize
{
    const SEP = " | ";
    public function describe();
}

class File implements Describable
{
    public function __construct(private $n, private $s)
    {
    }

    public function name()
    {
        return self::PREFIX . $this->n;
    }

    public function size()
    {
        return $this->s . static::UNIT;
    }

    public function describe()
    {
        return $this->name() . self::SEP . $this->size();
    }
}

interface Countable2
{
    public function total();
}

class Pair implements HasName, Countable2
{
    public function name()
    {
        return "pair";
    }

    public function total()
    {
        return 2;
    }
}

$f = new File("a.txt", 12);
echo $f->describe(), "\n";
echo "File instanceof HasName: ", ($f instanceof HasName ? "yes" : "no"), "\n";
echo "File instanceof HasSize: ", ($f instanceof HasSize ? "yes" : "no"), "\n";
echo "File instanceof Describable: ", ($f instanceof Describable ? "yes" : "no"), "\n";
echo "File instanceof Countable2: ", ($f instanceof Countable2 ? "yes" : "no"), "\n";

$p = new Pair;
echo $p->name(), " ", $p->total(), "\n";
echo "Pair instanceof Describable: ", ($p instanceof Describable ? "yes" : "no"), "\n";

echo "constants: ", HasName::PREFIX, " ", Describable::PREFIX, " ", Describable::UNIT, " ", File::SEP, "\n";
echo "via object: ", $f::UNIT, "\n";

abstract class Partial implements Describable
{
    public function name()
    {
        return "partial";
    }
}
class Completed extends Partial
{
    public function size()
    {
        return 0;
    }
    public function describe()
    {
        return $this->name() . "/" . $this->size();
    }
}
echo (new Completed)->describe(), "\n";

function takes_name(HasName $n)
{
    return $n->name();
}
echo takes_name($f), " ", takes_name($p), "\n";
