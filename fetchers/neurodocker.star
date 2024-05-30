def append_all(lst, items):
    if type(items) == "string":
        lst.append(items)
    else:
        for item in items:
            lst.append(item)

    return lst

def install_dependencies(ret, template, pkg_manager):
    if pkg_manager == "apt":
        pkgs = append_all([], template["dependencies"]["apt"])
        for pkg in pkgs:
            ret["directives"].append(("install", pkg))

        if "debs" in template["dependencies"]:
            for pkg in template["dependencies"]["debs"]:
                ret["directives"].append(("install", pkg))
    elif pkg_manager == "yum":
        pkgs = append_all([], template["dependencies"]["yum"])
        for pkg in pkgs:
            ret["directives"].append(("install", pkg))

        if "rpms" in template["dependencies"]:
            for pkg in template["dependencies"]["rpms"]:
                ret["directives"].append(("install", pkg))

    return ""

def install(ctx, pkgs):
    for pkg in pkgs:
        ctx["directives"].append(("install", pkg))
    return ""

def create_neurodocker_package(name, template, pkg_manager, params):
    ret = {
        "directives": [],
    }
    context = {
        # Old versions of NeuroDocker used the top level with the package name.
        # New versions just use self.
        "__top_level": name,

        # Functions.
        "install_dependencies": lambda: install_dependencies(ret, template, pkg_manager),
        "install": lambda pkgs: install(ret, pkgs),

        # Installation path.
        "install_path": "/install",

        # Not sure why these are needed but they break stuff if their missing.
        "curl_opts": "",
        "binaries_url": "",
        "pkg_manager": pkg_manager,
    }

    if "urls" in template:
        context["urls"] = template["urls"]

    for k in params:
        context[k] = params[k]

    if "arguments" in template:
        if "required" in template["arguments"]:
            for require in template["arguments"]["required"]:
                if require not in context:
                    return error("missing required parameter: " + require)

        if "optional" in template["arguments"]:
            optional = template["arguments"]["optional"]
            for optName in optional:
                if optName in context:
                    continue
                context[optName] = eval_jinja2(optional[optName], **context)

    ret["directives"].append(("run", eval_jinja2(template["instructions"], **context)))

    for k in template["env"]:
        ret["directives"].append(("env", k + "=" + eval_jinja2(template["env"][k], **context)))

    return ret["directives"]

def get_neurodocker_package(url, branch, name, pkg_manager, params):
    repo = fetch_git(url)
    tree = repo.branch(branch)

    template = tree["neurodocker/templates/{}.yaml".format(name)]

    template = parse_yaml(template.read())

    if "generic" in template:
        template = template["generic"]

    if "method" in params:
        return create_neurodocker_package(name, template[params["method"]], pkg_manager, params)

    if "binaries" in template:
        return create_neurodocker_package(name, template["binaries"], pkg_manager, params)
    elif "source" in template:
        return create_neurodocker_package(name, template["source"], pkg_manager, params)
    return error("could not find binaries or source")
