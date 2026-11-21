<?php
class Api
{
    const DEFAULT_LIMIT = 10;

    public function query($table, $limit = self::DEFAULT_LIMIT, $order = "asc", ...$fields)
    {
        $out = $table . " limit " . $limit . " " . $order;
        foreach ($fields as $k => $f) {
            $out .= " [" . $k . ":" . $f . "]";
        }
        return $out;
    }

    public function swap(&$a, &$b)
    {
        [$a, $b] = [$b, $a];
    }

    public static function sum(int ...$nums)
    {
        $t = 0;
        foreach ($nums as $n) {
            $t += $n;
        }
        return $t;
    }

    public function append(array &$list, $v = "default")
    {
        $list[] = $v;
        return $this;
    }

    public function opt(?string $a = null, $b = [1, 2])
    {
        return ($a ?? "null") . " " . $b[1];
    }
}

$api = new Api;
echo $api->query("users"), "\n";
echo $api->query("users", 5), "\n";
echo $api->query("users", order: "desc"), "\n";
echo $api->query("users", 1, "asc", "id", "name"), "\n";
echo $api->query(...["posts", 2, "desc", "title"]), "\n";
echo $api->query("t", 3, "x", ...["a", "b"]), "\n";
echo $api->query(table: "named", limit: 7), "\n";
echo $api->query("tags", 1, "asc", extra: "named variadic"), "\n";

$x = 1;
$y = 2;
$api->swap($x, $y);
echo "swap: ", $x, $y, "\n";
echo "variadic sum: ", Api::sum(), " ", Api::sum(1, 2, 3), " ", Api::sum(...[4, 5]), "\n";
$list = [];
$api->append($list, "a")->append($list)->append(v: "named", list: $list);
foreach ($list as $v) {
    echo $v, " ";
}
echo "\n";
echo $api->opt(), " | ", $api->opt("x"), " | ", $api->opt(b: [0, 9]), "\n";

function side($v)
{
    echo "<", $v, ">";
    return $v;
}
echo $api->query(side("a"), side(1), side("o")), "\n";
$obj = $api;
$obj->swap($obj, $x);
echo "swapped object into scalar: ", $obj, " ", ($x instanceof Api ? "x is Api" : "no"), "\n";
