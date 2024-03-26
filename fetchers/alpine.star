"""
Alpine Linux Package Fetcher
"""

alpine_mirror = "https://dl-cdn.alpinelinux.org/alpine"

def split_maybe(s, split, count, default = ""):
    ret = []

    if s != None:
        tokens = s.split(split, count - 1)
        for tk in tokens:
            ret.append(tk)
        for i in range(count - len(tokens)):
            ret.append(default)
    else:
        for i in range(count):
            ret.append(default)

    return ret

def split_dict_maybe(d, key, split):
    if key in d:
        return d[key].split(split)
    else:
        return []

def opt(d, key, default = ""):
    if key in d:
        return d[key]
    else:
        return default

def parse_apk_index(contents):
    lines = contents.splitlines()

    ret = []
    ent = {}

    for line in lines:
        if len(line) == 0:
            ret.append(ent)
            ent = {}
        else:
            k, v = line.split(":", 1)
            ent[k] = v

    return ret

def parse_apk_name(ctx, s):
    pkg = ""
    version = ""
    if ">=" in s:
        pkg, version = s.split(">=", 1)
        version = ">" + version
    else:
        pkg, version = split_maybe(s, "=", 2)
    if ":" in pkg:
        namespace, pkg = pkg.split(":", 1)
        return ctx.name(namespace = namespace, name = pkg, version = version)
    else:
        return ctx.name(name = pkg, version = version)

def fetch_alpine_repository(ctx, url):
    resp = fetch_http(url + "/APKINDEX.tar.gz")
    apk_index = resp.read_archive(".tar.gz")["APKINDEX"]

    contents = parse_apk_index(apk_index.read())

    for ent in contents:
        pkg = ctx.add_package(ctx.name(
            name = ent["P"],
            version = ent["V"],
            architecture = ent["A"],
        ))

        pkg.set_description(ent["T"])
        pkg.set_license(ent["L"])
        pkg.set_size(int(ent["S"]))
        pkg.set_installed_size(int(ent["I"]))

        pkg.add_source(url = "{}/{}-{}.apk".format(url, pkg.name, pkg.version))

        pkg.add_metadata("url", opt(ent, "U"))
        pkg.add_metadata("origin", opt(ent, "o"))
        pkg.add_metadata("commit", opt(ent, "c"))
        pkg.add_metadata("maintainer", opt(ent, "m"))

        for depend in split_dict_maybe(ent, "D", " "):
            pkg.add_dependency(parse_apk_name(ctx, depend))

        for alias in split_dict_maybe(ent, "p", " "):
            pkg.add_alias(parse_apk_name(ctx, alias))

for version in ["v3.19"]:
    for repo in ["main", "community"]:
        for arch in ["x86_64"]:
            fetch_repo(
                fetch_alpine_repository,
                ("{}/{}/{}/{}".format(alpine_mirror, version, repo, arch),),
                distro = "alpine@{}".format(version),
            )
